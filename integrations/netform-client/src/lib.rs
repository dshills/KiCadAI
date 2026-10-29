//! A local subprocess client, independent of NetForm's GUI and simulation engine.
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::{
    io::{Read, Write},
    path::Path,
    process::{Command, Stdio},
    sync::mpsc,
    thread,
    time::{Duration, Instant},
};

pub const REQUEST_SCHEMA: &str = "netform.kicadai.request.v1";
pub const RESPONSE_SCHEMA: &str = "netform.kicadai.response.v1";
pub const MAX_REQUEST: usize = 64 * 1024;
pub const MAX_RESPONSE: usize = 2 * 1024 * 1024;

#[derive(Debug, Serialize)]
pub struct Request<'a> {
    pub schema: &'static str,
    pub id: &'a str,
    pub method: &'a str,
    pub params: Value,
}
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProtocolError {
    pub code: String,
    pub message: String,
}
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Response {
    pub schema: String,
    pub id: String,
    pub ok: bool,
    pub result: Option<Value>,
    pub error: Option<ProtocolError>,
}

pub fn decode_response(raw: &[u8], id: &str) -> Result<Response, String> {
    if raw.len() > MAX_RESPONSE {
        return Err("Adapter response exceeds 2 MiB".into());
    }
    let response: Response = serde_json::from_slice(raw).map_err(|e| e.to_string())?;
    if response.schema != RESPONSE_SCHEMA || response.id != id {
        return Err("Adapter schema or request identity differs".into());
    }
    if response.ok == response.error.is_some() || response.ok != response.result.is_some() {
        return Err("Contradictory adapter response envelope".into());
    }
    Ok(response)
}

/// Calls only a caller-selected executable with the fixed `adapter` command.
/// Do not invoke this synchronously on a GUI event thread. It may take up to 45s.
pub fn call(binary: &Path, id: &str, method: &str, params: Value) -> Result<Response, String> {
    if !binary.is_absolute() {
        return Err("Choose an absolute path to the KiCadAI executable".into());
    }
    if id.is_empty()
        || id.len() > 64
        || !id
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"_.-".contains(&b))
        || !params.is_object()
    {
        return Err("A bounded request ID and params object are required".into());
    }
    let raw = serde_json::to_vec(&Request {
        schema: REQUEST_SCHEMA,
        id,
        method,
        params,
    })
    .map_err(|e| e.to_string())?;
    if raw.len() > MAX_REQUEST {
        return Err("Adapter request exceeds 64 KiB".into());
    }
    let mut command = Command::new(binary);
    command
        .arg("adapter")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    for (key, _) in std::env::vars_os() {
        let upper = key.to_string_lossy().to_ascii_uppercase();
        if ["API_KEY", "TOKEN", "SECRET", "PASSWORD"]
            .iter()
            .any(|s| upper.contains(s))
        {
            command.env_remove(key);
        }
    }
    let mut child = command.spawn().map_err(|e| e.to_string())?;
    let stdin = child.stdin.take().ok_or("Adapter stdin unavailable")?;
    let stdout = child.stdout.take().ok_or("Adapter stdout unavailable")?;
    let stderr = child.stderr.take().ok_or("Adapter stderr unavailable")?;
    let (writer_tx, writer) = mpsc::sync_channel(1);
    thread::spawn(move || {
        let mut stdin = stdin;
        let _ = writer_tx.send(stdin.write_all(&raw));
    });
    let (output_tx, output) = mpsc::sync_channel(1);
    thread::spawn(move || {
        let mut raw = Vec::new();
        let result = stdout
            .take((MAX_RESPONSE + 1) as u64)
            .read_to_end(&mut raw)
            .map(|_| raw);
        let _ = output_tx.send(result);
    });
    let (errors_tx, errors) = mpsc::sync_channel(1);
    thread::spawn(move || {
        let mut raw = Vec::new();
        let result = stderr.take(65537).read_to_end(&mut raw).map(|_| raw);
        let _ = errors_tx.send(result);
    });
    let deadline = Instant::now() + Duration::from_secs(45);
    let status = loop {
        match child.try_wait() {
            Ok(Some(status)) => break status,
            Ok(None) if Instant::now() < deadline => thread::sleep(Duration::from_millis(10)),
            Ok(None) => {
                let _ = child.kill();
                let _ = child.wait();
                return Err("Adapter exceeded 45 seconds".into());
            }
            Err(error) => {
                let _ = child.kill();
                let _ = child.wait();
                return Err(error.to_string());
            }
        }
    };
    finish_writer(writer, deadline)?;
    let raw = receive(output, deadline)?;
    let diagnostic = receive(errors, deadline)?;
    if diagnostic.len() > 65536 {
        return Err("Adapter stderr exceeds 64 KiB".into());
    }
    let response = decode_response(&raw, id)?;
    if status.success() != response.ok {
        return Err("Adapter exit status disagrees with response".into());
    }
    Ok(response)
}

fn receive<T>(
    receiver: mpsc::Receiver<std::io::Result<T>>,
    deadline: Instant,
) -> Result<T, String> {
    receiver
        .recv_timeout(deadline.saturating_duration_since(Instant::now()))
        .map_err(|_| "Adapter stream closed or exceeded its deadline".to_string())?
        .map_err(|e| e.to_string())
}

fn finish_writer(
    receiver: mpsc::Receiver<std::io::Result<()>>,
    deadline: Instant,
) -> Result<(), String> {
    let result = receiver
        .recv_timeout(deadline.saturating_duration_since(Instant::now()))
        .map_err(|_| "Adapter input stream closed or exceeded its deadline".to_string())?;
    match result {
        Ok(()) => Ok(()),
        Err(error) if error.kind() == std::io::ErrorKind::BrokenPipe => Ok(()),
        Err(error) => Err(error.to_string()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn broken_stdin_pipe_does_not_mask_a_structured_response() {
        for kind in [
            std::io::ErrorKind::BrokenPipe,
            std::io::ErrorKind::PermissionDenied,
        ] {
            let (tx, rx) = mpsc::sync_channel(1);
            tx.send(Err(std::io::Error::from(kind))).unwrap();
            assert_eq!(
                finish_writer(rx, Instant::now() + Duration::from_secs(1)).is_ok(),
                kind == std::io::ErrorKind::BrokenPipe
            );
        }
    }
    #[test]
    fn rejects_incompatible_or_oversized_envelopes() {
        for raw in [
            r#"{"schema":"old","id":"a","ok":true,"result":{}}"#,
            r#"{"schema":"netform.kicadai.response.v1","id":"b","ok":true,"result":{}}"#,
            r#"{"schema":"netform.kicadai.response.v1","id":"a","ok":true,"error":{"code":"x","message":"x"}}"#,
            r#"{"schema":"netform.kicadai.response.v1","id":"a","ok":true,"result":{},"extra":1}"#,
        ] {
            assert!(decode_response(raw.as_bytes(), "a").is_err());
        }
        assert!(decode_response(&vec![b' '; MAX_RESPONSE + 1], "a").is_err());
        assert!(
            call(
                Path::new("kicadai"),
                "a",
                "capabilities",
                serde_json::json!({})
            )
            .is_err()
        );
    }
    #[test]
    fn native_adapter_round_trip() {
        let Ok(binary) = std::env::var("KICADAI_ADAPTER_BIN") else {
            return;
        };
        let response = call(
            Path::new(&binary),
            "cap-1",
            "capabilities",
            serde_json::json!({}),
        )
        .unwrap();
        assert!(response.ok);
        assert!(
            response.result.unwrap()["methods"]
                .as_array()
                .unwrap()
                .len()
                >= 7
        );
        let response = call(
            Path::new(&binary),
            "catalog-1",
            "catalog.search",
            serde_json::json!({"family":"resistor","limit":2}),
        )
        .unwrap();
        assert!(response.ok);
        assert_eq!(
            response.result.unwrap()["items"].as_array().unwrap().len(),
            2
        );
        let response = call(
            Path::new(&binary),
            "negative-1",
            "execute",
            serde_json::json!({}),
        )
        .unwrap();
        assert!(!response.ok);
        assert_eq!(response.error.unwrap().code, "UNSUPPORTED_METHOD");
    }
}

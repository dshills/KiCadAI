use std::path::Path;
fn main() -> Result<(), String> {
    let binary = std::env::args()
        .nth(1)
        .ok_or("Pass the absolute path to kicadai")?;
    let response = netform_kicadai_client::call(
        Path::new(&binary),
        "catalog-example",
        "catalog.search",
        serde_json::json!({"family":"resistor","limit":5}),
    )?;
    if !response.ok {
        return Err(response.error.unwrap().message);
    }
    println!(
        "{}",
        serde_json::to_string_pretty(&response.result.unwrap()).map_err(|e| e.to_string())?
    );
    Ok(())
}

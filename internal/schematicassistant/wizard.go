package schematicassistant

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrUnconfirmed = errors.New("specification not confirmed; progress retained; no project generated")

// Wizard persists each accepted answer. EOF/cancel never authorizes generation.
func Wizard(ctx context.Context, input io.Reader, prompts io.Writer, session Session, save func(Session) error) (Session, error) {
	if session.Schema != Schema {
		return session, errors.New("unsupported assistant session schema")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), 8192)
	ask := func(prompt string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := fmt.Fprintln(prompts, prompt); err != nil {
			return "", err
		}
		text, err := ReadLine(ctx, scanner)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", ErrUnconfirmed
			}
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		answer := strings.TrimSpace(text)
		if answer == "cancel" || answer == "quit" {
			return "", ErrUnconfirmed
		}
		return answer, nil
	}
	session.ConfirmationSHA256 = ""
	if err := save(session); err != nil {
		return session, err
	}
	for attempts := 0; attempts < 48; attempts++ {
		questions := Questions(session.Spec)
		if len(questions) > 0 {
			question := questions[0]
			answer, err := ask(question.Prompt)
			if err != nil {
				return session, err
			}
			if err = Set(&session.Spec, question.Field, answer); err != nil {
				if _, err = fmt.Fprintln(prompts, err); err != nil {
					return session, err
				}
				continue
			}
			if err = save(session); err != nil {
				return session, err
			}
			continue
		}
		measurements, err := Analyze(session.Spec)
		if err != nil {
			if _, printErr := fmt.Fprintf(prompts, "%s\nEnter a different input voltage to stay in this bounded lane.\n", err); printErr != nil {
				return session, printErr
			}
			answer, readErr := ask("Input voltage in V:")
			if readErr != nil {
				return session, readErr
			}
			if setErr := Set(&session.Spec, "input_v", answer); setErr != nil {
				continue
			}
			if err = save(session); err != nil {
				return session, err
			}
			continue
		}
		if _, err = fmt.Fprintf(prompts, "\nConfirm this exact specification:\n  name: %s\n  family: %s\n  VIN: %.9g V\n  R1: %.9g ohm\n  load: %.9g ohm (0 = explicitly unloaded)\n", session.Spec.Name, session.Spec.Family, *session.Spec.InputV, *session.Spec.R1, *session.Spec.Load); err != nil {
			return session, err
		}
		if session.Spec.R2 != nil {
			if _, err = fmt.Fprintf(prompts, "  R2: %.9g ohm\n", *session.Spec.R2); err != nil {
				return session, err
			}
		}
		if session.Spec.C != nil {
			if _, err = fmt.Fprintf(prompts, "  C1: %.9g F\n", *session.Spec.C); err != nil {
				return session, err
			}
		}
		for _, m := range measurements {
			if _, err = fmt.Fprintf(prompts, "  %s: %.9g %s\n", m.Name, m.Value, m.Unit); err != nil {
				return session, err
			}
		}
		answer, err := ask("Assumptions: ideal source, ideal passive components, DC/0→VIN step only; no hardware validation or fabrication approval. Type yes to generate, edit to change a field, or cancel:")
		if err != nil {
			return session, err
		}
		if answer == "edit" {
			field, e := ask("Field: name, family, input_v, r1_ohms, r2_ohms, capacitance_f, load_ohms")
			if e != nil {
				return session, e
			}
			value, e := ask("New value:")
			if e != nil {
				return session, e
			}
			if e = Set(&session.Spec, field, value); e != nil {
				if _, err = fmt.Fprintln(prompts, e); err != nil {
					return session, err
				}
			}
			if e = save(session); e != nil {
				return session, e
			}
			continue
		}
		if answer != "yes" {
			return session, ErrUnconfirmed
		}
		session, err = Confirm(session.Spec)
		if err != nil {
			return session, err
		}
		if err = save(session); err != nil {
			return session, err
		}
		return session, nil
	}
	return session, errors.New("question limit reached; resume the saved session")
}

// ReadLine lets a CLI return on interrupt even while terminal input is pending.
// A cancelled reader is not reused; the CLI ends and any pending read is released.
func ReadLine(ctx context.Context, scanner *bufio.Scanner) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	type line struct {
		text string
		err  error
	}
	done := make(chan line, 1)
	go func() {
		if scanner.Scan() {
			done <- line{text: scanner.Text()}
			return
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		done <- line{err: err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-done:
		return result.text, result.err
	}
}

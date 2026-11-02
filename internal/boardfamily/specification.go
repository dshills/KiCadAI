package boardfamily

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const SpecificationVersion = "board-specification-1"
const SpecificationAcknowledgement = "I reviewed the original request, resolved omissions and conflicts, and accept this exact configuration and all catalog conditions. This does not certify manufactured hardware."

// Specification is an editable proposal, never permission to generate a board.
// OriginalRequest is retained verbatim so a reviewer can catch AI omissions.
type Specification struct {
	Version         string   `json:"version"`
	Origin          string   `json:"origin"`
	OriginalRequest string   `json:"original_request"`
	Configuration   *Config  `json:"configuration"`
	Unresolved      []string `json:"unresolved"`
	ReviewNotes     []string `json:"review_notes"`
}

type SpecificationReview struct {
	Specification   Specification `json:"specification"`
	Catalog         []FamilySpec  `json:"catalog"`
	Electrical      *Electrical   `json:"electrical,omitempty"`
	Ready           bool          `json:"ready_to_confirm"`
	Issues          []string      `json:"issues"`
	SHA256          string        `json:"sha256"`
	Acknowledgement string        `json:"required_acknowledgement"`
}

// Confirmation is a reproducible review receipt, not a signature or identity proof.
type ConfirmedSpecification struct {
	Version         string        `json:"version"`
	Specification   Specification `json:"specification"`
	ReviewSHA256    string        `json:"review_sha256"`
	Acknowledgement string        `json:"acknowledgement"`
}

func decodeSpecificationJSON(r io.Reader, out any) error {
	b, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil {
		return err
	}
	if len(b) > 65536 {
		return errors.New("specification exceeds 65536 bytes")
	}
	if err = validateJSONDepth(b, 12, "specification"); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode(out)
}

func DecodeSpecification(r io.Reader) (Specification, error) {
	var s Specification
	err := decodeSpecificationJSON(r, &s)
	if err == nil {
		err = checkSpecification(s)
	}
	return s, err
}

func DecodeConfirmedSpecification(r io.Reader) (ConfirmedSpecification, error) {
	var c ConfirmedSpecification
	err := decodeSpecificationJSON(r, &c)
	if err == nil {
		_, err = VerifyConfirmedSpecification(c)
	}
	return c, err
}

func checkSpecification(s Specification) error {
	if s.Version != SpecificationVersion || (s.Origin != "manual" && s.Origin != "ai") {
		return errors.New("unsupported specification version or origin")
	}
	if len(s.OriginalRequest) > 2000 || (s.Origin == "ai" && strings.TrimSpace(s.OriginalRequest) == "") {
		return errors.New("AI specification requires the original request (maximum 2000 bytes)")
	}
	if s.Unresolved == nil || s.ReviewNotes == nil || len(s.Unresolved) > 64 || len(s.ReviewNotes) > 64 {
		return errors.New("unresolved and review_notes must be arrays of at most 64 items")
	}
	for _, xs := range [][]string{s.Unresolved, s.ReviewNotes} {
		for _, x := range xs {
			if strings.TrimSpace(x) == "" || len(x) > 4000 {
				return errors.New("empty or oversized specification note")
			}
		}
	}
	return nil
}

func ReviewSpecification(s Specification) (SpecificationReview, error) {
	r := SpecificationReview{Specification: s, Catalog: Catalog(), Issues: []string{}, Acknowledgement: SpecificationAcknowledgement}
	if err := checkSpecification(s); err != nil {
		return r, err
	}
	r.Issues = append(r.Issues, s.Unresolved...)
	if s.Configuration == nil {
		r.Issues = append(r.Issues, "Choose an explicit configuration before confirmation.")
	} else if e, err := Check(*s.Configuration); err != nil {
		r.Issues = append(r.Issues, err.Error())
	} else {
		r.Electrical = &e
	}
	r.Ready = len(r.Issues) == 0
	// Hash all displayed review content, including the current catalog and acknowledgement.
	b, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	sum := sha256.Sum256(b)
	r.SHA256 = hex.EncodeToString(sum[:])
	return r, nil
}

func ConfirmSpecification(s Specification, digest string) (ConfirmedSpecification, error) {
	r, err := ReviewSpecification(s)
	if err != nil {
		return ConfirmedSpecification{}, err
	}
	if !r.Ready {
		return ConfirmedSpecification{}, errors.New("specification has unresolved or unsupported requirements; review and edit it first")
	}
	if digest == "" || digest != r.SHA256 {
		return ConfirmedSpecification{}, errors.New("review fingerprint differs; review the exact specification again")
	}
	return ConfirmedSpecification{"confirmed-board-specification-1", s, digest, SpecificationAcknowledgement}, nil
}

func VerifyConfirmedSpecification(c ConfirmedSpecification) (Config, error) {
	if c.Version != "confirmed-board-specification-1" || c.Acknowledgement != SpecificationAcknowledgement {
		return Config{}, errors.New("explicit specification confirmation is required")
	}
	if _, err := ConfirmSpecification(c.Specification, c.ReviewSHA256); err != nil {
		return Config{}, err
	}
	return *c.Specification.Configuration, nil
}

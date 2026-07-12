package aiprovider

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"kicadai/internal/intentplanner"
)

func TestOpenAILiveBMP280Intent(t *testing.T) {
	if os.Getenv("KICADAI_OPENAI_LIVE_TEST") != "1" {
		t.Skip("set KICADAI_OPENAI_LIVE_TEST=1 to run the live provider test")
	}
	prompt, err := os.ReadFile("../../examples/ai/usb_c_bmp280_breakout/prompt.txt")
	if err != nil {
		t.Fatalf("read reference prompt: %v", err)
	}
	provider, err := NewOpenAIProvider(OpenAIOptionsFromEnvironment())
	if err != nil {
		t.Fatalf("configure provider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := provider.GenerateIntent(ctx, GenerateRequest{
		Prompt:            string(prompt),
		CapabilityContext: BMP280ReferenceCapabilityContext,
		OutputSchemaName:  "kicadai_bmp280_intent_v1",
		OutputSchema:      BMP280ReferenceIntentEnvelopeSchema(),
		SchemaVersion:     EnvelopeSchemaV1,
		Attempt:           1,
	})
	if err != nil {
		t.Fatalf("generate reference intent: %v (cause %T: %v)", err, errors.Unwrap(err), errors.Unwrap(err))
	}
	request, issues := DecodeIntent(result.IntentJSON)
	if len(issues) != 0 {
		t.Fatalf("provider intent issues = %#v", issues)
	}
	plan := intentplanner.Plan(request)
	if plan.Status != intentplanner.PlanStatusReady || plan.GeneratedRequest == nil {
		t.Fatalf("provider plan status=%s issues=%#v gaps=%#v", plan.Status, plan.Issues, plan.KnownGaps)
	}
}

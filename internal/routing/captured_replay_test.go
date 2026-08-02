package routing

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestCapturedRoutingRequest(t *testing.T) {
	path := strings.TrimSpace(os.Getenv("KICADAI_CAPTURED_ROUTING_REQUEST"))
	if path == "" {
		t.Skip("set KICADAI_CAPTURED_ROUTING_REQUEST to replay a captured routing request")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("decode captured routing request: %v", err)
	}
	if order := strings.TrimSpace(os.Getenv("KICADAI_CAPTURED_ROUTING_NET_ORDER")); order != "" {
		priority := len(request.Nets)
		byName := make(map[string]*Net, len(request.Nets))
		for index := range request.Nets {
			byName[request.Nets[index].Name] = &request.Nets[index]
		}
		for _, name := range strings.Split(order, ",") {
			if net := byName[strings.TrimSpace(name)]; net != nil {
				net.OrderFirst = true
				net.Priority = priority
				priority--
			}
		}
	}
	if limitText := strings.TrimSpace(os.Getenv("KICADAI_CAPTURED_ROUTING_SEARCH_NODE_LIMIT")); limitText != "" {
		limit, err := strconv.Atoi(limitText)
		if err != nil || limit <= 0 {
			t.Fatalf("invalid KICADAI_CAPTURED_ROUTING_SEARCH_NODE_LIMIT %q", limitText)
		}
		request.Rules.MaxSearchNodes = limit
	}
	result := RouteRequestContext(context.Background(), request)
	if result.Status != StatusRouted {
		var failed []Route
		for _, route := range result.Routes {
			if route.Status == RouteStatusFailed {
				failed = append(failed, route)
			}
		}
		t.Fatalf("captured routing status=%s metrics=%#v failed=%#v", result.Status, result.Metrics, failed)
	}
}

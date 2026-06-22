package runtime

import "testing"

func TestParseTrafficStats(t *testing.T) {
	content := []byte(`{"stat":[
		{"name":"user>>>client@example.com>>>traffic>>>uplink","value":120},
		{"name":"user>>>client@example.com>>>traffic>>>downlink","value":380},
		{"name":"inbound>>>inbound-1>>>traffic>>>uplink","value":999}
	]}`)
	usage, err := parseTrafficStats(content)
	if err != nil {
		t.Fatal(err)
	}
	if usage["client@example.com"] != 500 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
}

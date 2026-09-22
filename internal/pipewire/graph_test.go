package pipewire

import "testing"

const dump = `[
  {"id": 30, "type": "PipeWire:Interface:Node", "info": {"props": {
    "node.name": "alsa_input.usb-X", "media.class": "Audio/Source",
    "alsa.card": 4, "audio.position": "[ MONO ]"}}},
  {"id": 31, "type": "PipeWire:Interface:Node", "info": {"props": {
    "node.name": "alsa_output.usb-X", "media.class": "Audio/Sink",
    "alsa.card": "4", "audio.position": "[ FL, FR ]"}}},
  {"id": 40, "type": "PipeWire:Interface:Node", "info": {"props": {
    "node.name": "other", "media.class": "Audio/Source", "alsa.card": 1}}},
  {"id": 50, "type": "PipeWire:Interface:Node", "info": {"props": {
    "node.name": "recorder"}}},
  {"id": 60, "type": "PipeWire:Interface:Link", "info": {
    "output-node-id": 30, "input-node-id": 50}}
]`

func TestParseDump(t *testing.T) {
	g, err := parseDump([]byte(dump))
	if err != nil {
		t.Fatal(err)
	}

	src, sink := g.CardNodes(4)
	if src == nil || src.Name != "alsa_input.usb-X" || src.Channels() != 1 {
		t.Errorf("source = %+v", src)
	}
	if sink == nil || sink.Name != "alsa_output.usb-X" || sink.Channels() != 2 {
		t.Errorf("sink = %+v", sink)
	}
	if src, sink := g.CardNodes(-1); src != nil || sink != nil {
		t.Error("card -1 must match nothing")
	}

	if !g.Linked("alsa_input.usb-X", "recorder") {
		t.Error("link not found")
	}
	if g.Linked("recorder", "alsa_input.usb-X") {
		t.Error("link direction ignored")
	}
}

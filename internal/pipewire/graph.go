package pipewire

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const (
	MediaSource = "Audio/Source"
	MediaSink   = "Audio/Sink"
)

type Node struct {
	ID         int
	Name       string
	MediaClass string
	Card       int // -1 if not alsa
	Positions  []string
}

func (n Node) Channels() int { return max(len(n.Positions), 1) }

type Link struct{ Out, In int }

type Graph struct {
	Nodes []Node
	Links []Link
}

func Dump() (*Graph, error) {
	out, err := exec.Command("pw-dump", "--no-colors").Output()
	if err != nil {
		return nil, fmt.Errorf("pw-dump: %w", err)
	}
	return parseDump(out)
}

type dumpObject struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Info struct {
		Props      map[string]any `json:"props"`
		OutputNode int            `json:"output-node-id"`
		InputNode  int            `json:"input-node-id"`
	} `json:"info"`
}

func parseDump(data []byte) (*Graph, error) {
	var objs []dumpObject
	if err := json.Unmarshal(data, &objs); err != nil {
		return nil, fmt.Errorf("pw-dump: %w", err)
	}
	g := &Graph{}
	for _, o := range objs {
		switch o.Type {
		case "PipeWire:Interface:Node":
			p := o.Info.Props
			g.Nodes = append(g.Nodes, Node{
				ID:         o.ID,
				Name:       str(p["node.name"]),
				MediaClass: str(p["media.class"]),
				Card:       card(p["alsa.card"]),
				Positions:  positions(p["audio.position"]),
			})
		case "PipeWire:Interface:Link":
			g.Links = append(g.Links, Link{Out: o.Info.OutputNode, In: o.Info.InputNode})
		}
	}
	return g, nil
}

func (g *Graph) CardNodes(card int) (src, sink *Node) {
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if card < 0 || n.Card != card {
			continue
		}
		switch {
		case n.MediaClass == MediaSource && src == nil:
			src = n
		case n.MediaClass == MediaSink && sink == nil:
			sink = n
		}
	}
	return src, sink
}

func (g *Graph) Node(name string) (*Node, bool) {
	for i := range g.Nodes {
		if g.Nodes[i].Name == name {
			return &g.Nodes[i], true
		}
	}
	return nil, false
}

func (g *Graph) Linked(out, in string) bool {
	o, ok1 := g.Node(out)
	i, ok2 := g.Node(in)
	if !ok1 || !ok2 {
		return false
	}
	for _, l := range g.Links {
		if l.Out == o.ID && l.In == i.ID {
			return true
		}
	}
	return false
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// number or string depending on pipewire version
func card(v any) int {
	switch c := v.(type) {
	case float64:
		return int(c)
	case string:
		var n int
		if _, err := fmt.Sscan(c, &n); err == nil {
			return n
		}
	}
	return -1
}

// "[ FL, FR ]"
func positions(v any) []string {
	return strings.FieldsFunc(str(v), func(r rune) bool {
		return r == '[' || r == ']' || r == ',' || r == ' '
	})
}

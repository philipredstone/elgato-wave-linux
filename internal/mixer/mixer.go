package mixer

import "strings"

type Settings map[string]int

func (s Settings) Pick(names []string) Settings {
	out := make(Settings, len(names))
	for _, n := range names {
		if v, ok := s[n]; ok {
			out[n] = v
		}
	}
	return out
}

type Field struct {
	Name    string
	Usage   string // empty = read-only
	Persist bool   // forgotten on power loss
	Format  func(raw int) string
	Parse   func(s string) (int, error)
}

func (f Field) Writable() bool { return f.Parse != nil }

type Mixer interface {
	Fields() []Field
	Read() (Settings, error)
	Write(Settings) error
	Meters() (left, right uint32, err error)
	Muted() (bool, error)
	Info() (Info, error)
}

type Info struct {
	Firmware string
	Serial   string
}

func Lookup(m Mixer, name string) (Field, bool) {
	for _, f := range m.Fields() {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

func PersistedFields(m Mixer) []string {
	var names []string
	for _, f := range m.Fields() {
		if f.Persist {
			names = append(names, f.Name)
		}
	}
	return names
}

func Describe(m Mixer, s Settings, names []string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		if f, ok := Lookup(m, n); ok {
			parts = append(parts, n+"="+f.Format(s[n]))
		}
	}
	return strings.Join(parts, " ")
}

package guardian

import (
	"errors"
	"log/slog"
	"maps"

	"github.com/philipredstone/elgato-wave-linux/internal/device"
	"github.com/philipredstone/elgato-wave-linux/internal/mixer"
	"github.com/philipredstone/elgato-wave-linux/internal/state"
)

// firmware forgets the gain on power loss, so we put it back
type settingsSync struct {
	store state.Store
	log   *slog.Logger

	node    string
	key     string
	saved   mixer.Settings
	pending mixer.Settings // seen once, needs a second poll
}

// TODO skip the write if nothing differs
func (s *settingsSync) restore(dev *device.Device) {
	if dev.Node == s.node {
		return
	}
	s.pending = nil
	mx := dev.Mixer
	if mx == nil {
		s.node, s.key = dev.Node, dev.StateKey()
		return
	}
	names := mixer.PersistedFields(mx)

	saved, err := s.store.Load(dev.StateKey())
	switch {
	case errors.Is(err, state.ErrNone):
	case err != nil:
		s.log.Error("load settings", "err", err)
		return
	default:
		if err := mx.Write(saved.Pick(names)); err != nil {
			s.log.Warn("restore settings", "err", err)
			return
		}
	}

	cur, err := mx.Read()
	if err != nil {
		s.log.Warn("read settings", "err", err)
		return
	}
	s.node, s.key = dev.Node, dev.StateKey()
	s.saved = cur.Pick(names)
	if err := s.store.Save(dev.StateKey(), s.saved); err != nil {
		s.log.Error("save settings", "err", err)
	}
	if saved != nil {
		s.log.Info("settings restored", "values", mixer.Describe(mx, s.saved, names))
	}
}

func (s *settingsSync) poll(dev *device.Device) {
	if dev.Node != s.node {
		s.restore(dev)
		return
	}
	if dev.Mixer != nil && dev.Present() {
		s.record(dev.Mixer)
	}
}

// two polls in a row, the block reads as garbage while power is going
func (s *settingsSync) record(mx mixer.Mixer) {
	names := mixer.PersistedFields(mx)
	all, err := mx.Read()
	if err != nil {
		return
	}
	cur := all.Pick(names)

	switch {
	case maps.Equal(cur, s.saved):
		s.pending = nil
		return
	case !maps.Equal(cur, s.pending):
		s.pending = cur
		return
	}

	var changed []string
	for _, n := range names {
		if cur[n] != s.saved[n] {
			changed = append(changed, n)
		}
	}
	if err := s.store.Save(s.key, cur); err != nil {
		s.log.Error("save settings", "err", err)
		return
	}
	s.saved, s.pending = cur, nil
	s.log.Info("settings saved", "changed", mixer.Describe(mx, cur, changed))
}

package state

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/philipredstone/elgato-wave-linux/internal/mixer"
)

type Store struct{ Dir string }

var ErrNone = errors.New("no saved settings")

func (s Store) path(key string) string { return filepath.Join(s.Dir, "settings-"+key) }

func (s Store) Load(key string) (mixer.Settings, error) {
	f, err := os.Open(s.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNone
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := mixer.Settings{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", s.path(key), name, err)
		}
		m[name] = v
	}
	return m, sc.Err()
}

func (s Store) Save(key string, m mixer.Settings) error {
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(m)) {
		fmt.Fprintf(&b, "%s %d\n", name, m[name])
	}

	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path(key))
}

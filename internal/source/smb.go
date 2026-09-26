package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudsoda/go-smb2"

	"github.com/meerhelm/jubilant-potato/internal/config"
	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// ErrAuthRequired means the SMB server wants a (different) user name and
// password.
var ErrAuthRequired = errors.New("login required")

const smbTimeout = 10 * time.Second

// SMBDial opens a session to host (":445" is added when no port is given).
// With an empty user it tries anonymous access, then the "guest" account.
func SMBDial(ctx context.Context, host, user, password string) (*smb2.Session, error) {
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "445")
	}
	ctx, cancel := context.WithTimeout(ctx, smbTimeout)
	defer cancel()

	users := []string{user}
	if user == "" {
		users = []string{"", "guest"}
	}
	var err error
	for _, u := range users {
		d := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: u, Password: password}}
		var s *smb2.Session
		if s, err = d.Dial(ctx, host); err == nil {
			return s, nil
		}
		if !isAuthError(err) {
			return nil, err
		}
	}
	return nil, ErrAuthRequired
}

func isAuthError(err error) bool {
	var re *smb2.ResponseError
	if !errors.As(err, &re) {
		return false
	}
	switch re.Code {
	case 0xC000006D, // STATUS_LOGON_FAILURE
		0xC0000022, // STATUS_ACCESS_DENIED
		0xC0000072, // STATUS_ACCOUNT_DISABLED (guest off)
		0xC000015B: // STATUS_LOGON_TYPE_NOT_GRANTED
		return true
	}
	return false
}

// SMBShares lists the user-visible shares of a server.
func SMBShares(ctx context.Context, host, user, password string) ([]string, error) {
	s, err := SMBDial(ctx, host, user, password)
	if err != nil {
		return nil, err
	}
	defer s.Logoff()
	names, err := s.WithContext(ctx).ListSharenames()
	if err != nil {
		if isAuthError(err) {
			return nil, ErrAuthRequired
		}
		return nil, err
	}
	var out []string
	for _, n := range names {
		if !strings.HasSuffix(n, "$") { // IPC$, ADMIN$, C$...
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}

// SMBDirs lists subfolders of dir ("" is the share root, "/" separated).
func SMBDirs(ctx context.Context, host, share, dir, user, password string) ([]string, error) {
	s, err := SMBDial(ctx, host, user, password)
	if err != nil {
		return nil, err
	}
	defer s.Logoff()
	fs, err := s.WithContext(ctx).Mount(share)
	if err != nil {
		if isAuthError(err) {
			return nil, ErrAuthRequired
		}
		return nil, err
	}
	defer fs.Umount()
	infos, err := fs.WithContext(ctx).ReadDir(smbPath(dir))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, fi := range infos {
		if fi.IsDir() && !strings.HasPrefix(fi.Name(), ".") {
			out = append(out, fi.Name())
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out, nil
}

// smbPath converts "a/b" to the share-relative form go-smb2 expects.
func smbPath(p string) string {
	return strings.ReplaceAll(strings.Trim(p, "/"), "/", `\`)
}

// smbSource browses a folder on an SMB share with one subfolder per system.
type smbSource struct {
	cfg config.Source

	mu    sync.Mutex
	sess  *smb2.Session
	share *smb2.Share
}

func newSMB(c config.Source) (*smbSource, error) {
	if c.Host == "" || c.Share == "" {
		return nil, fmt.Errorf("source %q: host and share are required", c.Name)
	}
	return &smbSource{cfg: c}, nil
}

func (s *smbSource) Name() string { return s.cfg.Name }
func (s *smbSource) Kind() string { return "SMB" }

// withShare runs f on a mounted share, reconnecting once if the cached
// session died (e.g. the handheld slept).
func (s *smbSource) withShare(ctx context.Context, f func(*smb2.Share) error) error {
	for attempt := 0; ; attempt++ {
		fs, err := s.mount(ctx)
		if err != nil {
			return err
		}
		err = f(fs.WithContext(ctx))
		if err == nil || attempt > 0 || ctx.Err() != nil || isAuthError(err) || errors.Is(err, iofs.ErrNotExist) {
			return err
		}
		s.reset()
	}
}

func (s *smbSource) mount(ctx context.Context) (*smb2.Share, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.share != nil {
		return s.share, nil
	}
	sess, err := SMBDial(ctx, s.cfg.Host, s.cfg.Username, s.cfg.Password)
	if err != nil {
		return nil, err
	}
	fs, err := sess.Mount(s.cfg.Share)
	if err != nil {
		sess.Logoff()
		if isAuthError(err) {
			return nil, ErrAuthRequired
		}
		return nil, err
	}
	s.sess, s.share = sess, fs
	return fs, nil
}

func (s *smbSource) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.share != nil {
		s.share.Umount()
		s.sess.Logoff()
	}
	s.sess, s.share = nil, nil
}

// dir joins the configured root with a relative folder.
func (s *smbSource) dir(rel string) string {
	return strings.Trim(strings.Trim(s.cfg.Path, "/")+"/"+strings.Trim(rel, "/"), "/")
}

func (s *smbSource) Systems(ctx context.Context) ([]System, error) {
	var out []System
	if len(s.cfg.Systems) > 0 {
		for id, paths := range s.cfg.Systems {
			if sys, ok := platform.SystemByID(id); ok {
				out = append(out, System{ID: id, Key: strings.Join(paths, "\n"), Label: sys.Name})
			}
		}
		sortSystems(out)
		return out, nil
	}
	var infos []smbEntry
	err := s.withShare(ctx, func(fs *smb2.Share) error {
		var err error
		infos, err = readDir(fs, s.dir(""))
		return err
	})
	if err != nil {
		return nil, err
	}
	byID := map[string]int{}
	for _, fi := range infos {
		if !fi.dir {
			continue
		}
		sys, ok := platform.MatchSystem(fi.name)
		if !ok {
			continue
		}
		if i, ok := byID[sys.ID]; ok {
			out[i].Key += "\n" + fi.name
			continue
		}
		byID[sys.ID] = len(out)
		out = append(out, System{ID: sys.ID, Key: fi.name, Label: sys.Name})
	}
	sortSystems(out)
	return out, nil
}

type smbEntry struct {
	name string
	size int64
	dir  bool
}

func readDir(fs *smb2.Share, dir string) ([]smbEntry, error) {
	infos, err := fs.ReadDir(smbPath(dir))
	if err != nil {
		return nil, err
	}
	out := make([]smbEntry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, smbEntry{name: fi.Name(), size: fi.Size(), dir: fi.IsDir()})
	}
	return out, nil
}

func (s *smbSource) Games(ctx context.Context, sys System) ([]Game, error) {
	psys, _ := platform.SystemByID(sys.ID)
	var out []Game
	for _, rel := range strings.Split(sys.Key, "\n") {
		dir := s.dir(rel)
		var infos []smbEntry
		err := s.withShare(ctx, func(fs *smb2.Share) error {
			var err error
			infos, err = readDir(fs, dir)
			return err
		})
		if err != nil {
			return nil, err
		}
		for _, fi := range infos {
			if fi.dir || strings.HasPrefix(fi.name, ".") || !psys.Accepts(fi.name) {
				continue
			}
			file := dir + "/" + fi.name
			out = append(out, Game{
				Name:   displayName(fi.name),
				File:   fi.name,
				Size:   fi.size,
				URL:    "smb://" + s.cfg.Host + "/" + s.cfg.Share + "/" + strings.TrimPrefix(file, "/"),
				System: sys.ID,
				Open: func(ctx context.Context, offset int64) (io.ReadCloser, int64, error) {
					return s.open(ctx, file, offset)
				},
			})
		}
	}
	sortGames(out)
	return out, nil
}

// open starts reading a file at offset and reports its full size.
func (s *smbSource) open(ctx context.Context, file string, offset int64) (io.ReadCloser, int64, error) {
	var f *smb2.File
	var size int64
	err := s.withShare(ctx, func(fs *smb2.Share) error {
		var err error
		if f, err = fs.Open(smbPath(file)); err != nil {
			return err
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		size = fi.Size()
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return err
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return f, size, nil
}

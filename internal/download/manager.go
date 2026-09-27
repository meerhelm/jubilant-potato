// Package download runs the download queue.
package download

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/meerhelm/jubilant-potato/internal/platform"
	"github.com/meerhelm/jubilant-potato/internal/source"
)

type State int

const (
	Queued State = iota
	Downloading
	Extracting
	Done
	Failed
	Canceled
)

// Job is one queued download.
type Job struct {
	ID      int
	Game    source.Game
	Dest    string // destination directory
	Extract bool

	done, total atomic.Int64

	mu     sync.Mutex
	state  State
	err    error
	cancel context.CancelFunc
}

// Info is a point-in-time copy of a job for the UI.
type Info struct {
	ID          int
	Name        string
	State       State
	Done, Total int64
	Err         error
}

func (j *Job) info() Info {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Info{ID: j.ID, Name: j.Game.Name, State: j.state, Done: j.done.Load(), Total: j.total.Load(), Err: j.err}
}

func (j *Job) setState(s State, err error) {
	j.mu.Lock()
	j.state, j.err = s, err
	j.mu.Unlock()
}

// Manager downloads jobs one at a time in the background.
type Manager struct {
	client *http.Client
	queue  chan *Job

	mu     sync.Mutex
	jobs   []*Job
	nextID int
}

func NewManager(client *http.Client) *Manager {
	m := &Manager{client: client, queue: make(chan *Job, 1024)}
	go m.worker()
	return m
}

// Enqueue schedules a download of g into dest.
func (m *Manager) Enqueue(g source.Game, dest string, extract bool) {
	m.mu.Lock()
	m.nextID++
	j := &Job{ID: m.nextID, Game: g, Dest: dest, Extract: extract}
	j.total.Store(g.Size)
	m.jobs = append(m.jobs, j)
	m.mu.Unlock()
	m.queue <- j
}

// Jobs returns a snapshot of all jobs, oldest first.
func (m *Manager) Jobs() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, len(m.jobs))
	for i, j := range m.jobs {
		out[i] = j.info()
	}
	return out
}

// Active reports the number of queued or running jobs.
func (m *Manager) Active() int {
	n := 0
	for _, j := range m.Jobs() {
		if j.State < Done {
			n++
		}
	}
	return n
}

// Pending reports whether a download of url is queued or running.
func (m *Manager) Pending(url string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		j.mu.Lock()
		p := j.Game.URL == url && j.state < Done
		j.mu.Unlock()
		if p {
			return true
		}
	}
	return false
}

// Cancel stops a queued or running job.
func (m *Manager) Cancel(id int) {
	if j := m.find(id); j != nil {
		j.mu.Lock()
		if j.state < Done {
			j.state = Canceled
			if j.cancel != nil {
				j.cancel()
			}
		}
		j.mu.Unlock()
	}
}

// Retry requeues a failed or canceled job.
func (m *Manager) Retry(id int) {
	if j := m.find(id); j != nil {
		j.mu.Lock()
		retry := j.state == Failed || j.state == Canceled
		if retry {
			j.state, j.err = Queued, nil
		}
		j.mu.Unlock()
		if retry {
			m.queue <- j
		}
	}
}

// ClearFinished drops completed jobs from the list.
func (m *Manager) ClearFinished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.jobs[:0]
	for _, j := range m.jobs {
		if j.info().State != Done {
			kept = append(kept, j)
		}
	}
	m.jobs = kept
}

func (m *Manager) find(id int) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

func (m *Manager) worker() {
	for j := range m.queue {
		ctx, cancel := context.WithCancel(context.Background())
		j.mu.Lock()
		if j.state != Queued {
			j.mu.Unlock()
			cancel()
			continue
		}
		j.state, j.cancel = Downloading, cancel
		j.mu.Unlock()

		err := m.run(ctx, j)
		canceled := ctx.Err() != nil
		cancel()
		switch {
		case err == nil:
			j.setState(Done, nil)
		case canceled:
			j.setState(Canceled, nil)
		default:
			j.setState(Failed, err)
		}
	}
}

func (m *Manager) run(ctx context.Context, j *Job) error {
	if err := os.MkdirAll(j.Dest, 0o755); err != nil {
		return err
	}
	final := filepath.Join(j.Dest, j.Game.File)
	part := final + ".part"

	if err := m.fetch(ctx, j, part); err != nil {
		return err
	}
	if j.Game.MD5 != "" {
		if err := checkMD5(part, j.Game.MD5); err != nil {
			os.Remove(part) // start over on retry
			return err
		}
	}
	if j.Game.Install != nil {
		j.setState(Extracting, nil)
		err := j.Game.Install(part)
		os.Remove(part)
		if err != nil {
			return fmt.Errorf("install: %w", err)
		}
		return nil
	}
	if !j.Extract || !strings.EqualFold(filepath.Ext(final), ".zip") {
		return os.Rename(part, final)
	}
	j.setState(Extracting, nil)
	if err := unzip(part, j.Dest); err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	return os.Remove(part)
}

// fetch downloads into part, resuming a previous partial download.
func (m *Manager) fetch(ctx context.Context, j *Job, part string) error {
	var offset int64
	if st, err := os.Stat(part); err == nil {
		offset = st.Size()
	}
	if j.Game.Open != nil {
		body, total, err := j.Game.Open(ctx, offset)
		if err != nil {
			return err
		}
		defer body.Close()
		if offset >= total && total > 0 {
			j.done.Store(offset)
			return nil
		}
		j.total.Store(total)
		return m.write(j, part, body, offset, os.O_CREATE|os.O_WRONLY|os.O_APPEND)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.Game.URL, nil)
	if err != nil {
		return err
	}
	for k, v := range j.Game.Header {
		req.Header[k] = v
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		flags |= os.O_APPEND
	case http.StatusOK:
		offset = 0
		flags |= os.O_TRUNC
	case http.StatusRequestedRangeNotSatisfiable:
		// Already complete.
		j.done.Store(offset)
		return nil
	default:
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	if resp.ContentLength > 0 {
		j.total.Store(offset + resp.ContentLength)
	}
	return m.write(j, part, resp.Body, offset, flags)
}

// write appends body to part after checking free space.
func (m *Manager) write(j *Job, part string, body io.Reader, offset int64, flags int) error {
	j.done.Store(offset)

	if total := j.total.Load(); total > 0 {
		need := uint64(total - offset)
		if j.Extract {
			need *= 2 // archive and its contents coexist while extracting
		}
		if free, err := platform.FreeBytes(j.Dest); err == nil && free < need {
			return fmt.Errorf("not enough space: need %d MB, free %d MB", need>>20, free>>20)
		}
	}

	f, err := os.OpenFile(part, flags, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, &progressReader{r: body, n: &j.done})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

type progressReader struct {
	r io.Reader
	n *atomic.Int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n.Add(int64(n))
	return n, err
}

func checkMD5(file, want string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, want) {
		return errors.New("download is corrupted (MD5 mismatch)")
	}
	return nil
}

func unzip(src, dest string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	root, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		target := filepath.Join(root, f.Name)
		if !strings.HasPrefix(target, root+string(os.PathSeparator)) {
			return errors.New("archive entry escapes destination: " + f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extractFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, rc)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

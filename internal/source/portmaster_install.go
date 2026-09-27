package source

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/meerhelm/jubilant-potato/internal/platform"
)

// pmTopLevel are the files besides scripts a port may keep at the top of
// its zip; harbourmaster moves them into the port's folder.
var pmTopLevel = map[string]bool{
	"cover.jpg": true, "cover.png": true, "gameinfo.xml": true, "port.json": true,
	"readme.md": true, "screenshot.jpg": true, "screenshot.png": true,
}

// installPort unpacks a port the way harbourmaster does: top-level scripts
// into the scripts folder, everything else into the ports folder. Files are
// made executable, as harbourmaster's chmod 777 does, since port zips often
// lose the bits. A port.json with the installed status and a signature in
// each script let PortMaster see the port as its own.
func installPort(archive string, dirs platform.PortMaster, p *pmPort) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	folder := p.folder()
	for _, f := range zr.File {
		name := strings.TrimPrefix(f.Name, "./")
		root, rel := dirs.Ports, name
		if !strings.Contains(strings.TrimSuffix(name, "/"), "/") && !f.FileInfo().IsDir() {
			switch lower := strings.ToLower(name); {
			case strings.HasSuffix(lower, ".sh"):
				root = dirs.Scripts
			case pmTopLevel[lower]:
				rel = folder + "/" + name
			}
		}
		target := filepath.Join(root, filepath.FromSlash(rel))
		if !strings.HasPrefix(target, filepath.Clean(root)+string(os.PathSeparator)) {
			return errors.New("archive entry escapes destination: " + f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extractPortFile(f, target); err != nil {
			return err
		}
	}

	files := map[string]string{"port.json": folder + "/port.json"}
	for _, it := range append(p.Items, p.ItemsOpt...) {
		script := strings.HasSuffix(strings.ToLower(it), ".sh")
		path := filepath.Join(dirs.Ports, it)
		if script {
			path = filepath.Join(dirs.Scripts, it)
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		files[it] = it // "2048/" keeps its slash, as in harbourmaster
		if script {
			if err := signScript(path, p.Name, it); err != nil {
				return err
			}
		}
	}
	return writePortInfo(filepath.Join(dirs.Ports, folder, "port.json"), p, files)
}

func extractPortFile(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	os.Remove(target) // a running binary can't be overwritten in place
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, rc)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// signScript puts harbourmaster's "# PORTMASTER: port.zip, Script.sh" line
// under the shebang; PortMaster uses it to tell which port a script is.
func signScript(path, port, script string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if !(strings.HasPrefix(strings.TrimSpace(l), "#") && strings.Contains(l, "PORTMASTER:")) {
			lines = append(lines, l)
		}
	}
	sig := "# PORTMASTER: " + port + ", " + script
	lines = append(lines[:1], append([]string{sig}, lines[1:]...)...)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o755)
}

// writePortInfo saves the catalog entry as harbourmaster does after an
// install: without the download source, with the installed status and the
// files that belong to the port.
func writePortInfo(path string, p *pmPort, files map[string]string) error {
	var info map[string]any
	if err := json.Unmarshal(p.raw, &info); err != nil {
		return err
	}
	delete(info, "source")
	info["name"] = p.Name
	info["status"] = map[string]string{"source": "PortMaster", "md5": p.Source.MD5, "status": "Installed"}
	info["files"] = files
	b, err := json.MarshalIndent(info, "", "    ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

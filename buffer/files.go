package buffer

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

func ensureDirectory(root *os.Root, name string) error {
	err := root.Mkdir(name, 0700)
	created := err == nil
	if err != nil && !errors.Is(err, os.ErrExist) {
		return errors.New("cannot create private buffer directory")
	}
	info, err := root.Lstat(name)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe buffer directory")
	}
	if created {
		parent, err := root.Open(".")
		if err != nil {
			return fmt.Errorf("cannot open buffer parent for sync: %w", err)
		}
		syncErr := parent.Sync()
		closeErr := parent.Close()
		if syncErr != nil {
			return fmt.Errorf("cannot sync new buffer directory entry: %w", syncErr)
		}
		if closeErr != nil {
			return fmt.Errorf("cannot close buffer parent: %w", closeErr)
		}
	}
	return nil
}

func (b *Buffer) prepareFiles() error {
	for _, dir := range append(append([]string{}, buckets...), "overflow") {
		if err := ensureDirectory(b.root, dir); err != nil {
			return err
		}
	}
	if err := b.safe(".buffer.lock", true); err != nil {
		return err
	}
	file, err := b.root.OpenFile(".buffer.lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if errors.Is(err, os.ErrExist) {
		return b.safe(".buffer.lock", false)
	}
	if err != nil {
		return errors.New("cannot create buffer lock")
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return errors.New("cannot sync buffer lock")
	}
	if err = file.Close(); err != nil {
		return errors.New("cannot close buffer lock")
	}
	return b.syncDirectory(".")
}

func (b *Buffer) safe(name string, missing bool) error {
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") {
		return errors.New("invalid buffer relative path")
	}
	components := strings.Split(name, "/")
	for index := range components {
		current := strings.Join(components[:index+1], "/")
		info, err := b.root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && missing && index == len(components)-1 {
			return nil
		}
		if err != nil {
			return errors.New("cannot inspect buffer path")
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return errors.New("unsafe buffer link or permissions")
		}
		if index < len(components)-1 {
			if !info.IsDir() {
				return errors.New("buffer parent is not a directory")
			}
		} else if !info.Mode().IsRegular() && !info.IsDir() {
			return errors.New("buffer entry is not a regular file or directory")
		}
	}
	return nil
}

func (b *Buffer) read(name string) ([]byte, bool, error) {
	if err := b.safe(name, true); err != nil {
		return nil, false, err
	}
	info, err := b.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, false, errors.New("cannot read buffer regular file")
	}
	data, err := b.root.ReadFile(name)
	if err != nil {
		return nil, false, errors.New("cannot read buffer file")
	}
	return data, true, nil
}

func (b *Buffer) atomicWrite(name string, data []byte) error {
	if err := b.safe(name, true); err != nil {
		return err
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return errors.New("cannot create buffer temporary name")
	}
	temp := name + ".tmp-" + hex.EncodeToString(suffix[:])
	file, err := b.root.OpenFile(temp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot create buffer temporary file")
	}
	defer func() { file.Close(); b.root.Remove(temp) }()
	var n int
	if b.writeTest != nil {
		n, err = b.writeTest(file, data)
	} else {
		n, err = file.Write(data)
	}
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return fmt.Errorf("cannot completely write buffer file: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("cannot sync buffer file: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("cannot close buffer file: %w", err)
	}
	if err = b.check("rename:" + name); err != nil {
		return fmt.Errorf("cannot replace buffer file: %w", err)
	}
	if err = b.root.Rename(temp, name); err != nil {
		return fmt.Errorf("cannot replace buffer file: %w", err)
	}
	return b.syncDirectory(path.Dir(name))
}

func (b *Buffer) syncDirectory(name string) error {
	if err := b.check("sync:" + name); err != nil {
		return fmt.Errorf("cannot sync buffer directory: %w", err)
	}
	dir, err := b.root.Open(name)
	if err != nil {
		return errors.New("cannot open buffer directory for sync")
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return fmt.Errorf("cannot sync buffer directory: %w", err)
	}
	return nil
}

func (b *Buffer) remove(name string) error {
	if err := b.safe(name, true); err != nil {
		return err
	}
	if err := b.check("remove:" + name); err != nil {
		return fmt.Errorf("cannot remove buffer file: %w", err)
	}
	err := b.root.Remove(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot remove buffer file: %w", err)
	}
	return b.syncDirectory(path.Dir(name))
}

func (b *Buffer) entries(bucket string) ([]string, error) {
	if err := b.safe(bucket, false); err != nil {
		return nil, err
	}
	dir, err := b.root.Open(bucket)
	if err != nil {
		return nil, errors.New("cannot open buffer bucket")
	}
	defer dir.Close()
	files, err := dir.ReadDir(-1)
	if err != nil {
		return nil, errors.New("cannot list buffer bucket")
	}
	names := []string{}
	for _, file := range files {
		name := file.Name()
		if file.Type()&os.ModeSymlink != 0 || file.IsDir() {
			return nil, errors.New("buffer record entry is not a regular file")
		}
		if strings.Contains(name, ".tmp-") {
			continue
		} // Owned interrupted atomic-write remnants are never records.
		if !strings.HasSuffix(name, ".yaml") {
			return nil, errors.New("unknown entry in buffer bucket")
		}
		hash := strings.TrimSuffix(name, ".yaml")
		if !validHash.MatchString(hash) {
			return nil, errors.New("invalid buffer record filename")
		}
		if err = b.safe(bucket+"/"+name, false); err != nil {
			return nil, err
		}
		names = append(names, hash)
	}
	sort.Strings(names)
	return names, nil
}

func (b *Buffer) count(status string) (int, error) {
	total := 0
	for _, bucket := range buckets {
		if status != "all" && status != bucket {
			continue
		}
		hashes, err := b.entries(bucket)
		if err != nil {
			return 0, err
		}
		total += len(hashes)
	}
	return total, nil
}

func (b *Buffer) list(status string, limit int) ([]Row, error) {
	rows := []Row{}
	for _, bucket := range buckets {
		if status != "all" && status != bucket {
			continue
		}
		hashes, err := b.entries(bucket)
		if err != nil {
			return nil, err
		}
		for _, hash := range hashes {
			record, err := b.readRecord(bucket, hash)
			if err != nil {
				return nil, err
			}
			if record == nil {
				return nil, errors.New("buffer record disappeared while locked")
			}
			message := record.Exception.Message
			token := strings.TrimLeft(message, "\n")
			if newline := strings.IndexByte(token, '\n'); newline >= 0 {
				token = token[:newline]
			}
			if token != "" && token != "0" {
				message = token
			} // PHP strtok(...) ?: original, including false "0".

			message = strings.Trim(message, " \t\n\r\v\x00")
			if len(message) > 160 {
				message = message[:160]
			}
			rows = append(rows, Row{Hash: hash, Status: bucket, FirstSeen: record.FirstSeen, LastSeen: record.LastSeen, Count: record.Count, Class: record.Exception.Class, MessageFirstLine: message, File: record.Exception.File, Line: record.Exception.Line, URL: record.Request.URL, Method: record.Request.Method})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].LastSeen > rows[j].LastSeen })
	if limit < 0 {
		limit = max(0, len(rows)+limit)
	} // PHP array_slice(...,0,negative) excludes tail.
	if limit < len(rows) {
		rows = rows[:limit]
	}
	return rows, nil
}

// I/O errors deliberately contain no YAML, journal or payload values.

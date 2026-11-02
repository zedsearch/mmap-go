// Copyright 2011 Evan Shaw. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// These tests are adapted from gommap: http://labix.org/gommap
// Copyright (c) 2010, Gustavo Niemeyer <gustavo@niemeyer.net>

package mmap

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

var testData = []byte("0123456789ABCDEF")
var testPath = filepath.Join(os.TempDir(), "testdata")

func init() {
	f := openFile(os.O_RDWR | os.O_CREATE | os.O_TRUNC)
	f.Write(testData)
	f.Close()
}

func openFile(flags int) *os.File {
	f, err := os.OpenFile(testPath, flags, 0644)
	if err != nil {
		panic(err.Error())
	}
	return f
}

func TestUnmap(t *testing.T) {
	f := openFile(os.O_RDONLY)
	defer f.Close()
	mmap, err := Map(f, RDONLY, 0)
	if err != nil {
		t.Errorf("error mapping: %s", err)
	}
	if err := mmap.Unmap(); err != nil {
		t.Errorf("error unmapping: %s", err)
	}
}

func TestReadWrite(t *testing.T) {
	f := openFile(os.O_RDWR)
	defer f.Close()
	mmap, err := Map(f, RDWR, 0)
	if err != nil {
		t.Errorf("error mapping: %s", err)
	}
	defer mmap.Unmap()
	if !bytes.Equal(testData, mmap) {
		t.Errorf("mmap != testData: %q, %q", mmap, testData)
	}

	mmap[9] = 'X'
	mmap.Flush()

	fileData, err := ioutil.ReadAll(f)
	if err != nil {
		t.Errorf("error reading file: %s", err)
	}
	if !bytes.Equal(fileData, []byte("012345678XABCDEF")) {
		t.Errorf("file wasn't modified")
	}

	// leave things how we found them
	mmap[9] = '9'
	mmap.Flush()
}

func TestProtFlagsAndErr(t *testing.T) {
	f := openFile(os.O_RDONLY)
	defer f.Close()
	if _, err := Map(f, RDWR, 0); err == nil {
		t.Errorf("expected error")
	}
}

func TestFlags(t *testing.T) {
	f := openFile(os.O_RDWR)
	defer f.Close()
	mmap, err := Map(f, COPY, 0)
	if err != nil {
		t.Errorf("error mapping: %s", err)
	}
	defer mmap.Unmap()

	mmap[9] = 'X'
	mmap.Flush()

	fileData, err := ioutil.ReadAll(f)
	if err != nil {
		t.Errorf("error reading file: %s", err)
	}
	if !bytes.Equal(fileData, testData) {
		t.Errorf("file was modified")
	}
}

// Test that we can map files from non-0 offsets
// The page size on most Unixes is 4KB, but on Windows it's 64KB
func TestNonZeroOffset(t *testing.T) {
	const pageSize = 65536

	// Create a 2-page sized file
	bigFilePath := filepath.Join(os.TempDir(), "nonzero")
	fileobj, err := os.OpenFile(bigFilePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		panic(err.Error())
	}

	bigData := make([]byte, 2*pageSize, 2*pageSize)
	fileobj.Write(bigData)
	fileobj.Close()

	// Map the first page by itself
	fileobj, err = os.OpenFile(bigFilePath, os.O_RDONLY, 0)
	if err != nil {
		panic(err.Error())
	}
	m, err := MapRegion(fileobj, pageSize, RDONLY, 0, 0)
	if err != nil {
		t.Errorf("error mapping file: %s", err)
	}
	m.Unmap()
	fileobj.Close()

	// Map the second page by itself
	fileobj, err = os.OpenFile(bigFilePath, os.O_RDONLY, 0)
	if err != nil {
		panic(err.Error())
	}
	m, err = MapRegion(fileobj, pageSize, RDONLY, 0, pageSize)
	if err != nil {
		t.Errorf("error mapping file: %s", err)
	}
	err = m.Unmap()
	if err != nil {
		t.Error(err)
	}

	m, err = MapRegion(fileobj, pageSize, RDONLY, 0, 1)
	if err == nil {
		t.Error("expect error because offset is not multiple of page size")
	}

	fileobj.Close()
}

func TestAnonymousMapping(t *testing.T) {
	const size = 4 * 1024

	// Make an anonymous region
	mem, err := MapRegion(nil, size, RDWR, ANON, 0)
	if err != nil {
		t.Fatalf("failed to allocate memory for buffer: %v", err)
	}

	// Check memory writable
	for i := 0; i < size; i++ {
		mem[i] = 0x55
	}

	// And unmap it
	err = mem.Unmap()
	if err != nil {
		t.Fatalf("failed to unmap memory for buffer: %v", err)
	}
}

func TestMapRegionWholeFileFromOffset(t *testing.T) {
	pageSize := os.Getpagesize()
	path := filepath.Join(t.TempDir(), "offset")
	data := make([]byte, 2*pageSize+10)
	data[pageSize] = 'A'
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	m, err := MapRegion(f, -1, RDONLY, 0, int64(pageSize))
	if err != nil {
		t.Fatalf("error mapping: %v", err)
	}
	if len(m) != pageSize+10 {
		t.Errorf("len = %d, want %d", len(m), pageSize+10)
	}
	if m[0] != 'A' {
		t.Errorf("m[0] = %q, want 'A'", m[0])
	}
	if err := m.Unmap(); err != nil {
		t.Fatal(err)
	}

	if _, err := MapRegion(f, -1, RDONLY, 0, int64(4*pageSize)); err == nil {
		t.Error("expected error for offset beyond end of file")
	}
}

func TestAddrLen(t *testing.T) {
	m, err := MapRegion(nil, 64, RDWR, ANON, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Unmap()
	addr, n := m.addrLen()
	if addr == 0 || n != 64 {
		t.Fatalf("addrLen = (%#x, %d)", addr, n)
	}
	if err := m.Lock(); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := m.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if err := m.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

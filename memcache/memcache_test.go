/*
Copyright 2011 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package memcache provides a client for the memcached cache server.
package memcache

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

var testServers = []string{"localhost:11211", "localhost:11212"}

func setup(t *testing.T, servers []string) {
	for _, server := range servers {
		c, err := net.Dial("tcp", server)
		if err != nil {
			t.Logf("no server running at %s", server)
		} else {
			c.Write([]byte("flush_all\r\n"))
			c.Close()
		}
	}
}

func TestLocalhost(t *testing.T) {
	setup(t, testServers)
	testWithClient(t, New(testServers...))
}

// Run the memcached binary as a child process and connect to its unix socket.
func TestUnixSocket(t *testing.T) {
	sock := fmt.Sprintf("/tmp/test-gomemcache-%d.sock", os.Getpid())
	cmd := exec.Command("memcached", "-s", sock)
	if err := cmd.Start(); err != nil {
		t.Logf("skipping test; couldn't find memcached")
		return
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	// Wait a bit for the socket to appear.
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(time.Duration(25*i) * time.Millisecond)
	}

	testWithClient(t, New(sock))
}

func checkErr(t *testing.T, c MemcacheClient, err error, format string, args ...interface{}) {
	if err != nil {
		t.Fatalf(format, args...)
	}
}
func mustSet(t *testing.T, c MemcacheClient, it *Item) {
	if err := c.Set(it); err != nil {
		t.Fatalf("failed to Set %#v: %v", *it, err)
	}
}

func testSetWithClient(t *testing.T, c MemcacheClient) {
	foo := &Item{Key: "foo", Value: []byte("fooval"), Flags: 123}
	err := c.Set(foo)
	checkErr(t, c, err, "first set(foo): %v", err)
	err = c.Set(foo)
	checkErr(t, c, err, "second set(foo): %v", err)
}

func testGetWithClient(t *testing.T, c MemcacheClient) {
	it, err := c.Get("foo")
	checkErr(t, c, err, "get(foo): %v", err)
	if it.Key != "foo" {
		t.Errorf("get(foo) Key = %q, want foo", it.Key)
	}
	if string(it.Value) != "fooval" {
		t.Errorf("get(foo) Value = %q, want fooval", string(it.Value))
	}
	if it.Flags != 123 {
		t.Errorf("get(foo) Flags = %v, want 123", it.Flags)
	}
}

func testAddWithClient(t *testing.T, c MemcacheClient) {
	bar := &Item{Key: "bar", Value: []byte("barval")}
	err := c.Add(bar)
	checkErr(t, c, err, "first add(foo): %v", err)
	if err := c.Add(bar); err != ErrNotStored {
		t.Fatalf("second add(foo) want ErrNotStored, got %v", err)
	}
}

func testGetMultiWithClient(t *testing.T, c MemcacheClient) {
	m, err := c.GetMulti([]string{"foo", "bar"})
	checkErr(t, c, err, "GetMulti: %v", err)
	if g, e := len(m), 2; g != e {
		t.Errorf("GetMulti: got len(map) = %d, want = %d", g, e)
	}
	if _, ok := m["foo"]; !ok {
		t.Fatalf("GetMulti: didn't get key 'foo'")
	}
	if _, ok := m["bar"]; !ok {
		t.Fatalf("GetMulti: didn't get key 'bar'")
	}
	if g, e := string(m["foo"].Value), "fooval"; g != e {
		t.Errorf("GetMulti: foo: got %q, want %q", g, e)
	}
	if g, e := string(m["bar"].Value), "barval"; g != e {
		t.Errorf("GetMulti: bar: got %q, want %q", g, e)
	}
}

func testDeleteWithClient(t *testing.T, c MemcacheClient) {
	err := c.Delete("foo")
	checkErr(t, c, err, "Delete: %v", err)
	_, err = c.Get("foo")
	if err != ErrCacheMiss {
		t.Errorf("post-Delete want ErrCacheMiss, got %v", err)
	}
}

func testIncrDecrWithClient(t *testing.T, c MemcacheClient) {
	mustSet(t, c, &Item{Key: "num", Value: []byte("42")})
	n, err := c.Increment("num", 8)
	checkErr(t, c, err, "Increment num + 8: %v", err)
	if n != 50 {
		t.Fatalf("Increment num + 8: want=50, got=%d", n)
	}
	n, err = c.Decrement("num", 49)
	checkErr(t, c, err, "Decrement: %v", err)
	if n != 1 {
		t.Fatalf("Decrement 49: want=1, got=%d", n)
	}
	err = c.Delete("num")
	checkErr(t, c, err, "delete num: %v", err)
	n, err = c.Increment("num", 1)
	if err != ErrCacheMiss {
		t.Fatalf("increment post-delete: want ErrCacheMiss, got %v", err)
	}
	mustSet(t, c, &Item{Key: "num", Value: []byte("not-numeric")})
	n, err = c.Increment("num", 1)
	if err == nil || !strings.Contains(err.Error(), "client error") {
		t.Fatalf("increment non-number: want client error, got %v", err)
	}
}

func testStatsWithClient(t *testing.T, c MemcacheClient) {
	stats, err := c.Stats()
	checkErr(t, c, err, "Stats: %v", err)
	if n := len(stats); err != nil || n == 0 {
		t.Errorf("Stats: didn't get stats")
	}
}

func testWithClient(t *testing.T, c MemcacheClient) {

	testSetWithClient(t, c)

	testGetWithClient(t, c)

	testAddWithClient(t, c)

	testGetMultiWithClient(t, c)

	testDeleteWithClient(t, c)

	testIncrDecrWithClient(t, c)

	testStatsWithClient(t, c)
}

func BenchmarkOnItem(b *testing.B) {
	fakeServer, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		b.Fatal("Could not open fake server: ", err)
	}
	defer fakeServer.Close()
	go func() {
		for {
			if c, err := fakeServer.Accept(); err == nil {
				go func() { io.Copy(ioutil.Discard, c) }()
			} else {
				return
			}
		}
	}()

	addr := fakeServer.Addr()
	c := New(addr.String())
	if _, err := c.getConn(addr); err != nil {
		b.Fatal("failed to initialize connection to fake server")
	}

	item := Item{Key: "foo"}
	dummyFn := func(_ *Client, _ *bufio.ReadWriter, _ *Item) error { return nil }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.onItem(&item, dummyFn)
	}
}

package raft

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"
)

const (
	TT_INMEM = iota
	TT_MAX
)

func NewTestTransport(ttype int, addr string) (string, LoopbackTransport) {
	var lt LoopbackTransport
	var err error
	switch ttype {
	case TT_INMEM:
		addr, lt = NewInmemTransport(addr)
		if err != nil {
			panic(fmt.Sprintf("Cannot create NewInmemUnixgramTransport: %v", err))
		}
	default:
		panic("Unknown transport type")
	}
	return addr, lt
}

func TestTransport_StartStop(t *testing.T) {
	for ttype := 0; ttype < TT_MAX; ttype++ {
		addr, trans := NewTestTransport(ttype, "")
		if addr == "" || trans == nil {
			t.Fatalf("No address / transport returned")
		}
		trans.Close()
	}
}

func TestTransport_AppendEntries(t *testing.T) {
	for ttype := 0; ttype < TT_MAX; ttype++ {
		addr1, trans1 := NewTestTransport(ttype, "")
		defer trans1.Close()
		rpcCh := trans1.Consumer()

		// Make the RPC request
		args := AppendEntriesRequest{
			Term:         10,
			Leader:       []byte("cartman"),
			PrevLogEntry: 100,
			PrevLogTerm:  4,
			Entries: []*Log{
				&Log{
					Index: 101,
					Term:  4,
					Type:  LogNoop,
				},
			},
			LeaderCommitIndex: 90,
		}
		resp := AppendEntriesResponse{
			Term:    4,
			LastLog: 90,
			Success: true,
		}

		// Listen for a request
		go func() {
			select {
			case rpc := <-rpcCh:
				// Verify the command
				req := rpc.Command.(*AppendEntriesRequest)
				if !reflect.DeepEqual(req, &args) {
					t.Fatalf("command mismatch: %#v %#v", *req, args)
				}
				rpc.Respond(&resp, nil)

			case <-time.After(200 * time.Millisecond):
				t.Fatalf("timeout")
			}
		}()

		// Transport 2 makes outbound request
		addr2, trans2 := NewTestTransport(ttype, "")
		defer trans2.Close()

		trans1.Connect(addr2, trans2)
		trans2.Connect(addr1, trans1)

		var out AppendEntriesResponse
		if err := trans2.AppendEntries(trans1.LocalAddr(), &args, &out); err != nil {
			t.Fatalf("err: %v", err)
		}

		// Verify the response
		if !reflect.DeepEqual(resp, out) {
			t.Fatalf("command mismatch: %#v %#v", resp, out)
		}
	}
}

func TestTransport_AppendEntriesPipeline(t *testing.T) {
	for ttype := 0; ttype < TT_MAX; ttype++ {
		addr1, trans1 := NewTestTransport(ttype, "")
		defer trans1.Close()
		rpcCh := trans1.Consumer()

		// Make the RPC request
		args := AppendEntriesRequest{
			Term:         10,
			Leader:       []byte("cartman"),
			PrevLogEntry: 100,
			PrevLogTerm:  4,
			Entries: []*Log{
				&Log{
					Index: 101,
					Term:  4,
					Type:  LogNoop,
				},
			},
			LeaderCommitIndex: 90,
		}
		resp := AppendEntriesResponse{
			Term:    4,
			LastLog: 90,
			Success: true,
		}

		// Listen for a request
		go func() {
			for i := 0; i < 10; i++ {
				select {
				case rpc := <-rpcCh:
					// Verify the command
					req := rpc.Command.(*AppendEntriesRequest)
					if !reflect.DeepEqual(req, &args) {
						t.Fatalf("command mismatch: %#v %#v", *req, args)
					}
					rpc.Respond(&resp, nil)

				case <-time.After(200 * time.Millisecond):
					t.Fatalf("timeout")
				}
			}
		}()

		// Transport 2 makes outbound request
		addr2, trans2 := NewTestTransport(ttype, "")
		defer trans2.Close()

		trans1.Connect(addr2, trans2)
		trans2.Connect(addr1, trans1)

		pipeline, err := trans2.AppendEntriesPipeline(trans1.LocalAddr())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		defer pipeline.Close()
		for i := 0; i < 10; i++ {
			out := new(AppendEntriesResponse)
			if _, err := pipeline.AppendEntries(&args, out); err != nil {
				t.Fatalf("err: %v", err)
			}
		}

		respCh := pipeline.Consumer()
		for i := 0; i < 10; i++ {
			select {
			case ready := <-respCh:
				// Verify the response
				if !reflect.DeepEqual(&resp, ready.Response()) {
					t.Fatalf("command mismatch: %#v %#v", &resp, ready.Response())
				}
			case <-time.After(200 * time.Millisecond):
				t.Fatalf("timeout")
			}
		}
	}
}

func TestTransport_RequestVote(t *testing.T) {
	for ttype := 0; ttype < TT_MAX; ttype++ {
		addr1, trans1 := NewTestTransport(ttype, "")
		defer trans1.Close()
		rpcCh := trans1.Consumer()

		// Make the RPC request
		args := RequestVoteRequest{
			Term:         20,
			Candidate:    []byte("butters"),
			LastLogIndex: 100,
			LastLogTerm:  19,
		}
		resp := RequestVoteResponse{
			Term:    100,
			Peers:   []byte("blah"),
			Granted: false,
		}

		// Listen for a request
		go func() {
			select {
			case rpc := <-rpcCh:
				// Verify the command
				req := rpc.Command.(*RequestVoteRequest)
				if !reflect.DeepEqual(req, &args) {
					t.Fatalf("command mismatch: %#v %#v", *req, args)
				}

				rpc.Respond(&resp, nil)

			case <-time.After(200 * time.Millisecond):
				t.Fatalf("timeout")
			}
		}()

		// Transport 2 makes outbound request
		addr2, trans2 := NewTestTransport(ttype, "")
		defer trans2.Close()

		trans1.Connect(addr2, trans2)
		trans2.Connect(addr1, trans1)

		var out RequestVoteResponse
		if err := trans2.RequestVote(trans1.LocalAddr(), &args, &out); err != nil {
			t.Fatalf("err: %v", err)
		}

		// Verify the response
		if !reflect.DeepEqual(resp, out) {
			t.Fatalf("command mismatch: %#v %#v", resp, out)
		}
	}
}

func TestTransport_InstallSnapshot(t *testing.T) {
	for ttype := 0; ttype < TT_MAX; ttype++ {
		addr1, trans1 := NewTestTransport(ttype, "")
		defer trans1.Close()
		rpcCh := trans1.Consumer()

		// Make the RPC request
		args := InstallSnapshotRequest{
			Term:         10,
			Leader:       []byte("kyle"),
			LastLogIndex: 100,
			LastLogTerm:  9,
			Peers:        []byte("blah blah"),
			Size:         10,
		}
		resp := InstallSnapshotResponse{
			Term:    10,
			Success: true,
		}

		// Listen for a request
		go func() {
			select {
			case rpc := <-rpcCh:
				// Verify the command
				req := rpc.Command.(*InstallSnapshotRequest)
				if !reflect.DeepEqual(req, &args) {
					t.Fatalf("command mismatch: %#v %#v", *req, args)
				}

				// Try to read the bytes
				buf := make([]byte, 10)
				rpc.Reader.Read(buf)

				// Compare
				if bytes.Compare(buf, []byte("0123456789")) != 0 {
					t.Fatalf("bad buf %v", buf)
				}

				rpc.Respond(&resp, nil)

			case <-time.After(200 * time.Millisecond):
				t.Fatalf("timeout")
			}
		}()

		// Transport 2 makes outbound request
		addr2, trans2 := NewTestTransport(ttype, "")
		defer trans2.Close()

		trans1.Connect(addr2, trans2)
		trans2.Connect(addr1, trans1)

		// Create a buffer
		buf := bytes.NewBuffer([]byte("0123456789"))

		var out InstallSnapshotResponse
		if err := trans2.InstallSnapshot(trans1.LocalAddr(), &args, &out, buf); err != nil {
			t.Fatalf("err: %v", err)
		}

		// Verify the response
		if !reflect.DeepEqual(resp, out) {
			t.Fatalf("command mismatch: %#v %#v", resp, out)
		}
	}
}

func TestTransport_EncodeDecode(t *testing.T) {
	for ttype := 0; ttype < TT_MAX; ttype++ {
		_, trans1 := NewTestTransport(ttype, "")
		defer trans1.Close()

		local := trans1.LocalAddr()
		enc := trans1.EncodePeer(local)
		dec := trans1.DecodePeer(enc)

		if dec != local {
			t.Fatalf("enc/dec fail: %v %v", dec, local)
		}
	}
}

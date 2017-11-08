package storage

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
)

func init() {
	log.Root().SetHandler(log.CallerFileHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(true)))))
}

func TestResourceHandler(t *testing.T) {

	cfg := &node.DefaultConfig
	cfg.IPCPath = "geth.ipc"
	stack, err := node.New(cfg)
	if err != nil {
		t.Fatalf("ouch %v", cfg)
	}
	err = stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		fullNode, err := eth.New(ctx, &eth.DefaultConfig)
		return fullNode, err
	})
	if err != nil {
		t.Fatalf("ouch %v", cfg)
	}
	stack.Start()
	defer stack.Stop()

	datadir, err := ioutil.TempDir("", "rh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(datadir)
	d, err := NewLocalDPA(datadir)
	if err != nil {
		t.Fatal(err)
	}
	d.Start()
	defer d.Stop()
	rh := NewResourceHandler(stack, d)
	_ = rh.NewResource("foo")
	rh.hasher.Reset()
	rh.hasher.Write([]byte("bar"))
	key := rh.hasher.Sum(nil)
	err = rh.Set(key, "foo")
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := d.Get(rh.Get("foo"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(chunk.SData, key) {
		t.Fatalf("Retrieved chunk data does not match input hash, expected %x, got %x", key, chunk.SData)
	}
}

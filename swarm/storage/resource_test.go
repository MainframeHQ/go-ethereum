package storage

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
)

func init() {
	log.Root().SetHandler(log.CallerFileHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(true)))))
}

func TestResourceHandler(t *testing.T) {
	datadir, err := ioutil.TempDir("", "rh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(datadir)
	cfg := &node.DefaultConfig
	cfg.IPCPath = "geth.ipc"
	cfg.DataDir = datadir
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

	d, err := NewLocalDPA(datadir)
	if err != nil {
		t.Fatal(err)
	}
	d.Start()
	defer d.Stop()
	for {
		_, err := os.Stat(stack.IPCEndpoint())
		if err == nil {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
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

//func makeDPA() (*DPA, error) {
//	hasher := MakeHashFunc("SHA256")
//	tmpdir, err := ioutil.TempDir("", "resourcesync")
//	if err != nil {
//		return nil, err
//	}
//	storeparams := NewStoreParams(tmpdir)
//	chunkerparams := NewChunkerParams()
//	localstore, err := NewLocalStore(hasher, storeparams)
//	if err != nil {
//		return nil, err
//	}
//	netstore := NewNetStore(hasher, localstore, nil, storeparams)
//	chunkstore := NewDpaChunkStore(localstore, netstore)
//	return NewDPA(chunkstore, chunkerparams), nil
//}

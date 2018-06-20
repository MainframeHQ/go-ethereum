package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/swarm/storage"
	"github.com/ethereum/go-ethereum/swarm/storage/mru"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"time"
)

// The append function is not a very good chat optimized solution for us. It basically requires you to dowload the entire chat log each time. We only need the diff.

type fakeBackend struct {
	blocknumber int64
}

func (f *fakeBackend) HeaderByNumber(context context.Context, name string, bigblock *big.Int) (*types.Header, error) {
	f.blocknumber++
	return &types.Header{
		Number: big.NewInt(f.blocknumber),
	}, nil
}

type owner bool

func (o *owner) ValidateOwner(name string, address common.Address) (bool, error) {
	return true, nil
}

func main() {
	setupLogging()

	// Create Dummy Handler Parameters
	mruParams := defaultHandlerParams()
	mruHandler, err := mru.NewHandler(mruParams)
	if err != nil {
		panic("mruHandler Error!")
	}

	// Setup a LocalStore & NetStore for Handler
	dir, err := ioutil.TempDir("", "mru")
	if err != nil {
		panic("dir Error!")
	}
	defer os.RemoveAll(dir)

	localStoreParams := storage.NewDefaultLocalStoreParams()
	localStoreParams.Init(dir)
	localStore, err := storage.NewLocalStore(localStoreParams, nil)
	if err != nil {
		panic("localStore Error!")
	}
	netStore := storage.NewNetStore(localStore, nil)

	mruHandler.SetStore(netStore)

	// 💪 Create a new root entry for a Mutable Resource
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	mruAddr, _, err := mruHandler.New(ctx, "awesomemru.eth", uint64(2))
	if err != nil {
		panic("mruHandler Errors!")
	}

	// 👀 print the address of the mutable resource root
	fmt.Printf("mru Root Addr = %s\n", mruAddr)

	// Create fileStore
	mapStore := storage.NewMapChunkStore()
	fileStoreParams := storage.NewFileStoreParams()
	fileStore := storage.NewFileStore(mapStore, fileStoreParams)

	// Create demoData to store
	demoData := bytes.NewBufferString("Testing 1234")

	// Persist demoData
	addr, wait, err := fileStore.Store(demoData, int64(demoData.Len()), false)
	if err != nil {
		panic("Store Error")
	}
	wait()

	// print the address of our persisted data
	fmt.Printf("persisted data addr = %s\n", addr)
	//chunk2, err := fileStore.Get(addr)
	//fmt.Printf("fileStore chunk = %s\n", chunk2)

	// print demoData before modifying it
	demoData.Reset()
	demoData.WriteString("567")
	//fmt.Printf("demoData = %s\n", demoData.Bytes())

	newAddr, wait, err := fileStore.Append(addr, demoData, false)
	if err != nil {
		panic("Append Error!")
	}
	wait()

	fmt.Printf("Appended data Address= %s\n", newAddr)
	// chunk, err := fileStore.Get(newAddr)
	// fmt.Printf("fileStore chunk = %s\n", chunk)

	// Retrieve the appended data to display it.
	reader, _ := fileStore.Retrieve(newAddr)
	quitC := make(chan bool)
	size, err := reader.Size(quitC)
	if err != nil {
		panic("reader.Size Error")
	}

	s := make([]byte, size)
	_, err = reader.Read(s)
	if err != io.EOF {
		panic("Read Error!")
	}

	fmt.Printf("🔴 Appended Data: %v \n", string(s))

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	mruAddr2, err := mruHandler.Update(ctx, "awesomemru.eth", newAddr)
	if err != nil {
		panic("Oh mruAddr2!")
	}
	fmt.Printf("mruAddr2 = %s\n", mruAddr2)

	// this fails because the block height hasn't progressed to be able to represent the update.
	// since we've essentially stubbed out our backend.
	addr3, data, err := mruHandler.GetContent("awesomemru.eth")

	if err != nil {
		fmt.Printf("err = %s\n", err)
		panic("Oh mruAddr2!")
	}
	fmt.Printf("addr3 = %s\n", addr3)
	fmt.Printf("data = %s\n", data)

}

func defaultHandlerParams() *mru.HandlerParams {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		panic("Oh No GenerateKey!")
	}
	qMaxPeriods := &mru.LookupParams{}
	signer := &mru.GenericSigner{PrivKey: privateKey}
	headerGetter := &fakeBackend{}
	var ownerValidator owner
	mruParams := &mru.HandlerParams{
		qMaxPeriods,
		signer,
		headerGetter,
		&ownerValidator,
	}
	return mruParams
}

func setupLogging() {
	// setup Logging
	hs := log.StreamHandler(os.Stderr, log.TerminalFormat(true))
	hf := log.LvlFilterHandler(log.LvlTrace, hs)
	h := log.CallerFileHandler(hf)
	log.Root().SetHandler(h)
}

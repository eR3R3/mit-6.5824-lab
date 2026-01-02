package kvsrv

import (
	"log"
	"sync"

	"6.5840/kvsrv1/rpc"
	"6.5840/labrpc"
	tester "6.5840/tester1"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

type KVServer struct {
	mu      sync.RWMutex
	KVStore map[string]Entry
}

func (kv *KVServer) AddRecord(key string, value string, version rpc.Tversion) {
	version++
	entry := &Entry{
		Value:   value,
		Version: version,
	}
	kv.KVStore[key] = *entry
}

func (kv *KVServer) UpdateRecord(key string, value string) bool {
	entryCurr, ok := kv.KVStore[key]
	if ok == false {
		log.Fatal("no entry found using the key when updating")
		return false
	}

	version := entryCurr.Version + 1

	entry := &Entry{
		Value:   value,
		Version: version,
	}

	kv.KVStore[key] = *entry
	return true
}

type Entry struct {
	Value   string
	Version rpc.Tversion
}

func MakeKVServer() *KVServer {
	kvStore := make(map[string]Entry, 1000)
	kv := &KVServer{
		KVStore: kvStore,
	}
	return kv
}

func (kv *KVServer) Get(args *rpc.GetArgs, reply *rpc.GetReply) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	key := args.Key

	entry, ok := kv.KVStore[key]
	if ok == true {
		reply.Version = entry.Version
		reply.Value = entry.Value
		reply.Err = rpc.OK
	} else {
		reply.Err = rpc.ErrNoKey
	}
}

func (kv *KVServer) Put(args *rpc.PutArgs, reply *rpc.PutReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	key := args.Key
	value := args.Value
	version := args.Version

	entry, ok := kv.KVStore[key]
	// if keyNotFound
	if ok == false {
		if version == 0 {
			kv.AddRecord(key, value, version)
			reply.Err = rpc.OK
		} else {
			reply.Err = rpc.ErrNoKey
		}
	} else {
		if version == entry.Version {
			kv.UpdateRecord(key, value)
			reply.Err = rpc.OK
		} else {
			reply.Err = rpc.ErrVersion
		}
	}
}

func (kv *KVServer) Kill() {
}

func StartKVServer(ends []*labrpc.ClientEnd, gid tester.Tgid, srv int, persister *tester.Persister) []tester.IService {
	server := MakeKVServer()
	// tester框架会自动处理RPC注册和网络，不需要手动启动HTTP服务器
	return []tester.IService{server}
}

package mr

import "fmt"
import "log"
import "net/rpc"
import "hash/fnv"

// KeyValue Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// 哈希，用于分桶
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	// 在二进制上按位相加，目的是在32位的机器上不会变成负数
	return int(h.Sum32() & 0x7fffffff)
}

// Worker 请求任务
// 执行map或者reduce任务
// 处理文件输入输出
// RPC返回任务状态
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {
}

func call(rpcName string, args any, reply any) bool {
	// 拿到socket address
	sockName := coordinatorSock()
	// conn到rpc服务器
	c, err := rpc.DialHTTP("unix", sockName)
	// 错误检查
	if err != nil {
		log.Fatal("dialing:", err)
	}
	// 结束conn
	defer c.Close()

	// call rpcName函数
	err = c.Call(rpcName, args, reply)
	// 错误检查
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}

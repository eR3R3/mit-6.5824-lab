package mr

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)
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
	Id := os.Getpid()
	reply := getTaskRPC(Id)
	for {
		switch reply.TaskInfo.TaskType {
		case MapTask:
			doMap(reply.TaskInfo, mapf)
		case ReduceTask:
			doReduce(reply.TaskInfo, reply.BucketNum, reply.MapNum, reducef)
		case WaitTask:
			time.Sleep(time.Millisecond * 500)
		case FinishedTask:
			return
		}
	}
}

func doMap(taskInfo Task, mapf func(string, string) []KeyValue) {
	file, err := os.Open(taskInfo.FileName)
	defer file.Close()
	if err != nil {
		log.Fatalf("err opening file in map")
	}

	content, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("err reading file")
	}

	allKVs := mapf(taskInfo.FileName, string(content))

	intermediate := make([][]KeyValue, taskInfo.NReduce)

	// 分桶
	for _, kv := range allKVs {
		key := kv.Key
		hashedKey := ihash(key)
		bucketNum := hashedKey % taskInfo.NReduce
		intermediate[bucketNum] = append(intermediate[bucketNum], kv)
	}

	// 写入文件
	for i := range taskInfo.NReduce {
		file, err := os.CreateTemp("", fmt.Sprintf("mr-%d-%d", taskInfo.TaskId, i))
		if err != nil {
			log.Fatal("Create Temp File Error")
		}

		encoder := json.NewEncoder(file)
		for kv := range intermediate[i] {
			_ = encoder.Encode(&kv)
		}
		_ = file.Close()
	}

	reportTaskDone(os.Getpid(), taskInfo.TaskId)
}

func doReduce(taskInfo Task, bucketNum int, mapNum int, reducef func(string, []string) string) {
	// pass in the responsible bucket
	// for read the buckets
	var allKV []KeyValue
	for i := range mapNum {
		fileName := fmt.Sprintf("mr-%d-%d", i, bucketNum)
		file, _ := os.Open(fileName)
		decoder := json.NewDecoder(file)

		var kv KeyValue
		for {
			err := decoder.Decode(&kv)
			if err != nil {
				break
			}
			allKV = append(allKV, kv)
		}
		_ = file.Close()
	}
	// sort
	sort.Slice(allKV, func(i, j int) bool {
		return allKV[i].Key < allKV[j].Key
	})

	start := 0
	end := 0

	// call reduce on the same key
	for i := 0; i < len(allKV); i++ {
		if allKV[i].Key == allKV[i-1].Key {
			end += 1
			continue
		}

		temp := make([]string, 0, 100)
		for j := start; j < end; j++ {
			temp = append(temp, allKV[j].Value)
		}

		res := reducef(allKV[i-1].Key, temp)
		file, _ := os.CreateTemp("", fmt.Sprintf("res-%d", bucketNum))
		_, _ = file.Write([]byte(res))
	}

	reportTaskDone(os.Getpid(), taskInfo.TaskId)
}

func getTaskRPC(id int) *GetTaskReply {
	args := GetTaskArgs{id}
	reply := &GetTaskReply{}
	call("GetTask", args, reply)
	return reply
}

func reportTaskDone(id int, taskId TaskId) {
	args := ReportTaskArgs{TaskId: taskId, WorkerId: id}
	reply := &ReportTaskReply{}
	call("ReportTask", args, reply)
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

package mr

import (
	"log"
	"sync"
	"time"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

// Coordinator 管理任务的状态
// 处理worker的RPC请求
// 检查是否超时

type TaskType string
type TaskId int

const (
	MapTask      TaskType = "map"
	ReduceTask   TaskType = "reduce"
	WaitTask     TaskType = "wait"
	FinishedTask TaskType = "finished"
)

type Status string

const (
	Idle       Status = "idle"
	InProgress Status = "in-progress"
	Completed  Status = "completed"
)

type Task struct {
	FileName  string
	nReduce   int
	status    Status
	taskType  TaskType
	StartTime time.Time
	TaskId    TaskId
}

func dummyTask() Task {
	return Task{
		TaskId: -1,
	}
}

func NewTask(taskType TaskType) Task {
	return Task{
		taskType: taskType,
	}
}

type Coordinator struct {
	mu          sync.Mutex
	tasks       []Task
	nReduce     int
	nMap        int
	mapCount    int
	reduceCount int
	files       []string
}

func (c *Coordinator) getTaskByTaskId(id TaskId) *Task {
	for _, task := range c.tasks {
		if task.TaskId == id {
			return &task
		}
	}
	return nil
}

func (c *Coordinator) GetTask(args GetTaskArgs, reply *GetTaskReply) error {
	c.checkTimeout()
	// 检查是否done
	mapStatus, mapTask := c.getMapTask()
	reduceStatus, reduceTask := c.getReduceTask()
	if mapStatus == Completed && reduceStatus == Completed {
		*reply = GetTaskReply{
			taskInfo: Task{
				taskType: FinishedTask,
			},
		}
		return nil
	}

	// 如果map没执行完分配map
	if mapStatus == Idle {
		mapTask.status = InProgress
		mapTask.StartTime = time.Now()
		*reply = GetTaskReply{
			taskInfo: mapTask,
		}
	}
	// 如果还有map in progress让他等一下
	if mapStatus == InProgress {
		*reply = GetTaskReply{
			taskInfo: Task{
				taskType: WaitTask,
			},
		}
	}
	// map执行完了分配reduce
	if mapStatus == Completed && reduceStatus == Idle {
		reduceTask.status = InProgress
		reduceTask.StartTime = time.Now()
		*reply = GetTaskReply{
			taskInfo: reduceTask,
		}
	}
	// 如果还有reduce in progress让他等一下
	if mapStatus == Completed && reduceStatus == InProgress {
		*reply = GetTaskReply{
			taskInfo: Task{
				taskType: WaitTask,
			},
		}
	}

	return nil
}

// checkTimeout 检查是否有超时任务加回任务列表
func (c *Coordinator) checkTimeout() {
	currTime := time.Now()
	for _, task := range c.tasks {
		diff := currTime.Sub(task.StartTime)
		if diff > 10*time.Second {
			task.status = Idle
		}
	}
}

// getMapTask 顺序检查所有map任务
// 有idle返回idle
// 没有idle返回in progress
// 没有in progress返回completed代表全部完成
func (c *Coordinator) getMapTask() (Status, Task) {
	progress := make([]Task, 0)
	// check有没有idle的task
	for _, task := range c.tasks {
		if task.taskType == MapTask {
			if task.status == Idle {
				return Idle, task
			} else if task.status == InProgress {
				progress = append(progress, task)
			}
		}
	}

	if len(progress) == 0 {
		return Completed, dummyTask()
	} else {
		return InProgress, progress[0]
	}
}

func (c *Coordinator) getReduceTask() (Status, Task) {
	progress := make([]Task, 0)
	// check有没有idle的task
	for _, task := range c.tasks {
		if task.taskType == ReduceTask {
			if task.status == Idle {
				return Idle, task
			} else if task.status == InProgress {
				progress = append(progress, task)
			}
		}
	}

	if len(progress) == 0 {
		return Completed, dummyTask()
	} else {
		return InProgress, progress[0]
	}
}

func (c *Coordinator) ReportTask(args ReportTaskArgs, reply *ReportTaskReply) error {
	// 更新状态
	task := c.getTaskByTaskId(args.taskId)
	if task == nil {
		log.Fatalf("no task found by taskId")
	}
	task.status = Completed
	*reply = ReportTaskReply{ok: true}
	return nil
}

func MakeCoordinator(files []string, nReduce int) *Coordinator {
	// add map tasks
	tasks := make([]Task, 0)
	for i, file := range files {
		currTask := Task{
			FileName: file,
			nReduce:  nReduce,
			status:   Idle,
			taskType: MapTask,
			TaskId:   TaskId(i),
		}
		tasks = append(tasks, currTask)
	}

	// add reduce tasks
	for i := range nReduce {
		currTask := Task{
			status:   Idle,
			nReduce:  nReduce,
			taskType: ReduceTask,
			TaskId:   TaskId(len(files) + i),
		}
		tasks = append(tasks, currTask)
	}

	c := Coordinator{
		tasks:       tasks,
		nReduce:     nReduce,
		nMap:        len(files),
		mapCount:    0,
		reduceCount: 0,
		files:       files,
	}

	c.server()
	return &c
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	err := rpc.Register(c)
	if err != nil {
		log.Fatal("register error:", err)
	}
	rpc.HandleHTTP()
	sockName := coordinatorSock()
	os.Remove(sockName)
	l, e := net.Listen("unix", sockName)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// Done main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	ret := false

	return ret
}

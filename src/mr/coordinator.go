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

// Task RPC要传的field要大写首字母
type Task struct {
	FileName  string
	NReduce   int
	Status    Status
	TaskType  TaskType
	StartTime time.Time
	TaskId    TaskId
}

func NewTask(taskType TaskType) Task {
	return Task{
		TaskType: taskType,
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
	for i, task := range c.tasks {
		if task.TaskId == id {
			// 不能直接给函数内的临时变量的指针
			return &c.tasks[i]
		}
	}
	return nil
}

func (c *Coordinator) GetTask(args GetTaskArgs, reply *GetTaskReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.checkTimeout()
	// 检查是否done
	mapStatus, mapTask := c.getMapTask()
	reduceStatus, reduceTask := c.getReduceTask()
	if mapStatus == Completed && reduceStatus == Completed {
		*reply = GetTaskReply{
			TaskInfo: Task{
				TaskType: FinishedTask,
			},
		}
		return nil
	}

	// 如果map没执行完分配map
	if mapStatus == Idle {
		mapTask.Status = InProgress
		mapTask.StartTime = time.Now()
		*reply = GetTaskReply{
			TaskInfo: *mapTask,
		}
		return nil
	}

	// 如果还有map in progress让他等一下
	if mapStatus == InProgress {
		*reply = GetTaskReply{
			TaskInfo: Task{
				TaskType: WaitTask,
			},
		}
		return nil
	}
	// map执行完了分配reduce
	if mapStatus == Completed && reduceStatus == Idle {
		reduceTask.Status = InProgress
		reduceTask.StartTime = time.Now()
		*reply = GetTaskReply{
			TaskInfo:  *reduceTask,
			BucketNum: c.reduceCount,
			MapNum:    len(c.files),
		}
		c.reduceCount++
		return nil
	}
	// 如果还有reduce in progress让他等一下
	if mapStatus == Completed && reduceStatus == InProgress {
		*reply = GetTaskReply{
			TaskInfo: Task{
				TaskType: WaitTask,
			},
		}
		return nil
	}

	return nil
}

// checkTimeout 检查是否有超时任务加回任务列表
func (c *Coordinator) checkTimeout() {
	currTime := time.Now()
	for i, task := range c.tasks {
		diff := currTime.Sub(task.StartTime)
		if diff > 10*time.Second && task.Status == InProgress {
			c.tasks[i].Status = Idle
		}
	}
}

// getMapTask 顺序检查所有map任务
// 有idle返回idle
// 没有idle返回in progress
// 没有in progress返回completed代表全部完成
func (c *Coordinator) getMapTask() (Status, *Task) {
	progress := make([]*Task, 0)
	// check有没有idle的task
	for i, task := range c.tasks {
		if task.TaskType == MapTask {
			if task.Status == Idle {
				return Idle, &c.tasks[i]
			} else if task.Status == InProgress {
				progress = append(progress, &c.tasks[i])
			}
		}
	}

	if len(progress) == 0 {
		return Completed, nil
	} else {
		return InProgress, progress[0]
	}
}

func (c *Coordinator) getReduceTask() (Status, *Task) {
	progress := make([]*Task, 0)
	// check有没有idle的task
	for i, task := range c.tasks {
		if task.TaskType == ReduceTask {
			if task.Status == Idle {
				return Idle, &c.tasks[i]
			} else if task.Status == InProgress {
				progress = append(progress, &c.tasks[i])
			}
		}
	}

	if len(progress) == 0 {
		return Completed, nil
	} else {
		return InProgress, progress[0]
	}
}

func (c *Coordinator) ReportTask(args ReportTaskArgs, reply *ReportTaskReply) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 更新状态
	task := c.getTaskByTaskId(args.TaskId)
	if task == nil {
		log.Fatalf("no task found by taskId")
	}
	task.Status = Completed
	*reply = ReportTaskReply{ok: true}
	return nil
}

func MakeCoordinator(files []string, nReduce int) *Coordinator {
	// add map tasks
	tasks := make([]Task, 0)
	for i, file := range files {
		currTask := Task{
			FileName: file,
			NReduce:  nReduce,
			Status:   Idle,
			TaskType: MapTask,
			TaskId:   TaskId(i),
		}
		tasks = append(tasks, currTask)
	}

	// add reduce tasks
	for i := range nReduce {
		currTask := Task{
			Status:   Idle,
			NReduce:  nReduce,
			TaskType: ReduceTask,
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

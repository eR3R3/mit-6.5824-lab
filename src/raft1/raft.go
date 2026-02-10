package raft

// The file raftapi/raft.go defines the inteselface that raft must
// expose to servers (or the tester), but see comments below for each
// of these functions for more details.
//
// Make() creates a new raft peer that implements the raft inteselface.

import (
	"6.5840/raftapi"
	"log"

	//	"bytes"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"6.5840/labgob"
	"6.5840/labrpc"
	"6.5840/tester1"
)

type NodeRole string

const (
	Follower  NodeRole = "follower"
	Leader    NodeRole = "leader"
	Candidate NodeRole = "candidate"
)

// 一个新的term就代表一轮新的尝试选举开始了，要清空下面东西
// votedFor和votesReceived
// 我给别人requestVote
// 别人给我requestVote
// 我给别人appendEntries
// 别人给我appendEntries

// 如果我是candidate，在requestVote，则变成follower
// 如果我是leader，在appendEntries，则变成follower

// Raft A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *tester.Persister   // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	currTerm int
	role     NodeRole

	log []Entry

	// 这个是为了他再次问的时候再次答复
	votedFor int
	// startElection非阻塞，这个用来统计目前term收到的票数
	votesReceived int

	electionTimeout time.Duration
	lastHeartBeat   time.Time

	lastAppliedIndex  int
	lastCommitedIndex int
}

func (self *Raft) resetTimeout() {
	self.lastHeartBeat = time.Now()
}

func (self *Raft) setRole(role NodeRole) {
	self.role = role
}

func (self *Raft) checkTerm(term int) bool {
	if term > self.currTerm {
		self.setRole(Follower)
		self.currTerm = term
		self.votedFor = -1
		return false
	}
	return true
}

type RequestVoteArgs struct {
	// Your data here (3A, 3B).
	Term            int
	CandidateId     int
	LastestLogTerm  int
	LastestLogIndex int
}

type RequestVoteReply struct {
	// Your data here (3A).
	Term        int
	VoteGranted bool
}

type AppendEntriesArgs struct {
	Term     int
	LeaderId int
	Entries  []Entry
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

type Entry struct {
	Term    int
	Index   int
	Command any
}

// Make
// the ports all servers are in peers[].
// this server's port is peers[me].
// all the servers' peers[] arrays have the same order.
// persister saves its persistent state and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *tester.Persister, applyCh chan raftapi.ApplyMsg) raftapi.Raft {
	self := &Raft{}
	self.peers = peers
	self.persister = persister
	self.me = me
	log.Printf("%v", self.me)
	self.votedFor = -1
	self.role = Follower
	self.electionTimeout = time.Duration(150+rand.Intn(150)) * time.Millisecond
	self.lastHeartBeat = time.Now()

	// initialize from state persisted before a crash
	self.readPersist(persister.ReadRaftState())

	// 插入一个dummy entry防止len-1的时候index out of range
	self.log = append(self.log, Entry{
		Term:    0,
		Index:   0,
		Command: 0,
	})

	// start ticker goroutine to start elections
	// leader: 过一段时间AppendEntries
	// follower: 检查是不是有election timeout
	go self.ticker()

	return self
}

func (self *Raft) broadCastAppendEntries(args *AppendEntriesArgs) {
	for i, _ := range self.peers {
		if i != self.me {
			go self.sendAppendEntries(i, args)
		}
	}
}

func (self *Raft) sendAppendEntries(server int, args *AppendEntriesArgs) {
	var reply AppendEntriesReply
	// 这个地方return ok了说明reply已经到了
	ok := self.peers[server].Call("Raft.AppendEntries", args, &reply)
	if !ok {
		return
	}
}

func (self *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	self.mu.Lock()
	defer self.mu.Unlock()
	// AppendEntries
	// 更新currTerm
	// 如果是旧的leader返回错误
	legit := self.checkTerm(args.Term)
	if legit == true {
		reply.Success = false
		reply.Term = self.currTerm
		return
	}

	if len(args.Entries) == 0 {
		// 这种情况下是heartbeat
		self.resetTimeout()
	}
	reply.Term = self.currTerm
	reply.Success = true
	// TODO: 加上正常的AppendEntries的情况
}

func (self *Raft) preElection() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.currTerm++
	self.role = Candidate
	self.votedFor = self.me
	self.lastHeartBeat = time.Now()
}

func (self *Raft) broadCastVote(args *RequestVoteArgs) {
	for i, _ := range self.peers {
		if i != self.me {
			// log.Printf("who is me, %v, receiver, %v", self.me, i)
			go self.sendRequestVote(i, args)
		}
	}
}

func (self *Raft) sendRequestVote(server int, args *RequestVoteArgs) {
	var reply RequestVoteReply
	// 这个地方return ok了说明reply已经到了
	ok := self.peers[server].Call("Raft.RequestVote", args, &reply)
	if !ok {
		return
	}
	// log.Printf("%v, %v", ok, reply)

	self.mu.Lock()
	defer self.mu.Unlock()
	log.Printf("I am %v, reply: %v, currTerm: %v", self.me, reply, self.currTerm)

	// 拿到reply后
	// 如果这个election timeout结束了，前一轮的选举失效了，直接忽略
	if reply.Term < self.currTerm {
		return
	}

	// 如果对面回给我的term比我的还大，直接变成follower, 更新term
	if self.checkTerm(reply.Term) == false {
		return
	}

	// 更新目前的voteCount
	self.votesReceived += 1
	if self.votesReceived > len(self.peers)/2 {
		self.role = Leader
	}
}

func (self *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// log.Printf("me, %v, I receive, %v", self.me, args)
	self.mu.Lock()
	defer self.mu.Unlock()

	// 判断request的term
	// 让上一个leader意识到他不是现在的leader
	if self.checkTerm(args.Term) == true {
		reply.VoteGranted = false
		reply.Term = self.currTerm
		return
	}
	// 判断last entry的term
	if args.LastestLogTerm < self.log[len(self.log)-1].Term {
		reply.VoteGranted = false
		reply.Term = self.currTerm
		return
	}

	// 判断last entry的index
	if args.LastestLogIndex < self.log[len(self.log)-1].Index {
		reply.VoteGranted = false
		reply.Term = self.currTerm
		return
	}

	// 如果都满足，并且我还没vote过
	if self.votedFor == -1 {
		reply.VoteGranted = true
		log.Printf("I am %v, I grant: %v", self.me, reply.VoteGranted)
		reply.Term = self.currTerm
		self.votedFor = args.CandidateId
	}
	// timer是用来保证leader没死的，我们现在投了一个leader，就timer reset了
	self.resetTimeout()
	return
}

func (self *Raft) ticker() {
	for self.killed() == false {

		// Your code here (3A)
		// leader: 过一段时间AppendEntries, 相当于发送heartbeat
		// follower: 检查是不是有election timeout
		if self.role == Leader {
			args := &AppendEntriesArgs{}
			args.LeaderId = self.me
			args.Term = self.currTerm
			self.broadCastAppendEntries(args)
		} else {
			if time.Now().Sub(self.lastHeartBeat) > self.electionTimeout {
				// 变成candidate, 要startElection了
				self.preElection()
				args := &RequestVoteArgs{}
				args.CandidateId = self.me
				args.Term = self.currTerm
				// if len(self.log) == 0 {
				// log.Fatal("no dummy entry")
				// }
				// log.Printf("machine num, %v, peers", self.me)
				args.LastestLogTerm = self.log[len(self.log)-1].Term
				args.LastestLogIndex = self.log[len(self.log)-1].Index
				self.broadCastVote(args)
			}
		}

		// 如果没有超时，过50 - 350ms之后再检查
		ms := 50 + (rand.Int63() % 300)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

// GetState
// currTerm, isLeader
func (self *Raft) GetState() (int, bool) {
	return self.currTerm, self.role == Leader
}

// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (self *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (3B).

	return index, term, isLeader
}

// Kill
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (self *Raft) Kill() {
	atomic.StoreInt32(&self.dead, 1)
	// Your code here, if desired.
}

func (self *Raft) killed() bool {
	z := atomic.LoadInt32(&self.dead)
	return z == 1
}

// restore previously persisted state.
func (self *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (3C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   self.xxx = xxx
	//   self.yyy = yyy
	// }
}

// how many bytes in Raft's persisted log?
func (self *Raft) PersistBytes() int {
	self.mu.Lock()
	defer self.mu.Unlock()
	return self.persister.RaftStateSize()
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (self *Raft) persist() {
	// Your code here (3C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(self.xxx)
	// e.Encode(self.yyy)
	// raftstate := w.Bytes()
	// self.persister.Save(raftstate, nil)
}

// Snapshot the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (self *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (3D).

}

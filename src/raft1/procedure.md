Leader Election

leader 
- send heartbeat
  - 150ms一次
  - 用一个空的AppendEntries RPC
- 如果自己heartbeat出现问题了
  - 他会收到candidate给他发的requestVote，如果term大于他的，就退位
  - resetElectionTimeout
  - 如果term小于他的，就取消这个vote，可能另外一个机器网络延迟

follower
- receive heartbeat
  - 重置计时器
  - 更新term
  - 只要收到了，说明leader已经产生了，不管做什么就中断然后重新变成follower


- election timeout(1.5-3s)的时间里没收到heartbeat并且没收到别人的requestVote，
- 变成candidate，直接requestVote
  - 从follower变成Candidate
  - currentTerm++
  - call所有机器的requestVote包括leader
  - 如果收到超过一半的, 成功，发送heartbeat
  - 如果没成功(split vote)，term++, 继续等待这个goroutine触发timeout
  - 5秒之内选出来


- 如果收到了别人的requestVote，说明变成了follower
  - 通过机制投票（还没投过的情况下）
    - args.Term < currentTerm 拒绝
    - 如果大于说明我是follower，更新currTerm继续判断
    - 等于说明我是candidate
    - 比完term之后比last entry的term和index，优先级term更大
  - vote完之后reset timeout
- 在Make()里面用一个background goroutine


不管收到什么append entries，只要term大于它，就resetElectionTimeout
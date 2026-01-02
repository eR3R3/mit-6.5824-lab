package lock

import (
	"6.5840/kvsrv1/rpc"
	"6.5840/kvtest1"
	"fmt"
)

type Lock struct {
	// IKVClerk is a go interface for k/v clerks: the interface hides
	// the specific Clerk type of ck but promises that ck supports
	// Put and Get.  The tester passes the clerk in when calling
	// MakeLock().
	ck  kvtest.IKVClerk
	key string
}

// MakeLock The tester calls MakeLock() and passes in a k/v clerk; your code can
// perform a Put or Get by calling lk.ck.Put() or lk.ck.Get().
//
// Use l as the key to store the "lock state" (you would have to decide
// precisely what the lock state is).
func MakeLock(ck kvtest.IKVClerk, l string) *Lock {
	lk := &Lock{
		ck:  ck,
		key: l,
	}

	return lk
}

func (lockWrapper *Lock) Acquire() {
	success := false
	for success == false {
		value, version, err := lockWrapper.ck.Get(lockWrapper.key)
		valWritten := kvtest.RandValue(8)
		if err == rpc.ErrNoKey {
			err = lockWrapper.ck.Put(lockWrapper.key, valWritten, 0)
			if err != rpc.OK {
				continue
			} else {
				success = true
			}
		} else {
			if value == "" {
				err = lockWrapper.ck.Put(lockWrapper.key, valWritten, version)
				if err == rpc.OK {
					success = true
				} else {
					if err == rpc.ErrMaybe {
						val, _ := lockWrapper.ReadUntilSuccess()
						if val != valWritten {
							continue
						} else {
							fmt.Println("?")
							success = true
						}
					}
				}
			} else {
				continue
			}
		}
	}
}

func (lockWrapper *Lock) Release() {
	_, version, _ := lockWrapper.ck.Get(lockWrapper.key)
	lockWrapper.CheckAndReput(version)
}

func (lockWrapper *Lock) CheckAndReput(currVersion rpc.Tversion) {
	success := false

	for success == false {
		//fmt.Println("stuck at CheckAndReput")
		_, version := lockWrapper.ReadUntilSuccess()
		if currVersion < version {
			success = true
		} else {
			lockWrapper.ck.Put(lockWrapper.key, "", version)
		}
	}
}

func (lockWrapper *Lock) ReadUntilSuccess() (string, rpc.Tversion) {
	success := false
	var value string
	var version rpc.Tversion
	var err rpc.Err

	for success == false {
		//fmt.Println("stuck at ReadUntilSuccess")
		value, version, err = lockWrapper.ck.Get(lockWrapper.key)
		if err != rpc.OK {
			continue
		} else {
			success = true
		}
	}

	return value, version
}

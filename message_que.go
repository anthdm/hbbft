package hbbft

import (
	"sync"
)

type messageTuple struct {
	to      uint64
	payload interface{}
}

type messageQue struct {
	que  []messageTuple
	lock sync.RWMutex
}

func newMessageQue() *messageQue {
	return &messageQue{
		que: []messageTuple{},
	}
}

func (q *messageQue) addMessage(msg interface{}, to uint64) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.que = append(q.que, messageTuple{to, msg})
}

func (q *messageQue) addMessages(msgs ...messageTuple) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.que = append(q.que, msgs...)
}

func (q *messageQue) addQue(que *messageQue) {
	newQue := make([]messageTuple, len(q.que)+que.len())
	copy(newQue, q.messages())
	copy(newQue, que.messages())

	q.lock.Lock()
	defer q.lock.Unlock()
	q.que = newQue
}

func (q *messageQue) len() int {
	q.lock.RLock()
	defer q.lock.RUnlock()
	return len(q.que)
}

func (q *messageQue) messages() []messageTuple {
	q.lock.RLock()
	msgs := q.que
	q.lock.RUnlock()

	q.lock.Lock()
	defer q.lock.Unlock()
	q.que = []messageTuple{}
	return msgs
}

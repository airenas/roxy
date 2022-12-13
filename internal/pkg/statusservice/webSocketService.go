package statusservice

import (
	"strings"
	"sync"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
)

// WsConn is interface for websocket handling in status service
type WsConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
	WriteJSON(v interface{}) error
}

// WSConnKeeper implements connection management
type WSConnKeeper struct {
	idConnectionMap map[string]map[WsConn]struct{}
	connectionIDMap map[WsConn]string
	mapLock         *sync.Mutex
	timeOut         time.Duration
}

// NewWSConnKeeper creates manager
func NewWSConnKeeper() *WSConnKeeper {
	res := &WSConnKeeper{}
	res.idConnectionMap = make(map[string]map[WsConn]struct{})
	res.connectionIDMap = make(map[WsConn]string)
	res.mapLock = &sync.Mutex{}
	res.timeOut = time.Minute * 30 // add max time limit for connection - if longer so sorry
	return res
}

// HandleConnection loops until connection active and save connection with provided ID as key
func (kp *WSConnKeeper) HandleConnection(conn WsConn) error {
	defer kp.deleteConnection(conn)
	defer conn.Close()
	readCh := make(chan string)
	go func() {
		defer close(readCh)
		defer goapp.Log.Debug().Msg("read routine ended")
		for {
			goapp.Log.Info().Msg("handleConnection")
			_, message, err := conn.ReadMessage()
			if err != nil {
				goapp.Log.Error().Err(err).Send()
				return
			}
			msg := strings.TrimSpace(string(message))
			goapp.Log.Debug().Str("msg", goapp.Sanitize(msg)).Msg("got msg")
			if msg != "" {
				readCh <- msg
			} else {
				time.Sleep(20 * time.Millisecond)
			}
		}
	}()

	ta := time.After(kp.timeOut)
loop:
	for {
		select {
		case <-ta:
			goapp.Log.Debug().Msg("conn timeouted")
			break loop
		case msg, ok := <-readCh:
			if !ok {
				goapp.Log.Debug().Msg("conn read closed?")
				break loop
			}
			kp.saveConnection(conn, msg)
			ta = time.After(kp.timeOut)
		}
	}
	goapp.Log.Info().Msg("handleConnection finish")
	return nil
}

func (kp *WSConnKeeper) deleteConnection(conn WsConn) {
	kp.mapLock.Lock()
	defer kp.mapLock.Unlock()
	kp.deleteConnectionNoSync(conn)
}

func (kp *WSConnKeeper) deleteConnectionNoSync(conn WsConn) {
	goapp.Log.Info().Msg("deleteConnection")
	id, found := kp.connectionIDMap[conn]
	if found {
		conns, found := kp.idConnectionMap[id]
		if found {
			delete(conns, conn)
			if len(conns) == 0 {
				delete(kp.idConnectionMap, id)
			}
		}
	}
	delete(kp.connectionIDMap, conn)
	goapp.Log.Info().Int("active", len(kp.connectionIDMap)).Msg("deleteConnection finish")
}

func (kp *WSConnKeeper) saveConnection(conn WsConn, id string) {
	goapp.Log.Info().Str("ID", id).Msg("saveConnection")
	kp.mapLock.Lock()
	defer kp.mapLock.Unlock()
	kp.deleteConnectionNoSync(conn)
	kp.connectionIDMap[conn] = id
	conns, found := kp.idConnectionMap[id]
	if !found {
		conns = map[WsConn]struct{}{}
		kp.idConnectionMap[id] = conns
	} else {
		goapp.Log.Info().Str("ID", id).Msg("found old ws connection")
	}
	conns[conn] = struct{}{}
	goapp.Log.Info().Int("active", len(kp.connectionIDMap)).Msg("saveConnection finish")
}

// GetConnections returns saved connections by provided id
func (kp *WSConnKeeper) GetConnections(id string) ([]WsConn, bool) {
	goapp.Log.Debug().Str("ID", id).Msgf("get ws connections")
	kp.mapLock.Lock()
	defer kp.mapLock.Unlock()
	cm, found := kp.idConnectionMap[id]
	if found {
		res := []WsConn{}
		for cm := range cm {
			res = append(res, cm)
		}
		return res, true
	}
	goapp.Log.Debug().Str("ID", id).Msgf("no ws connections")
	return nil, false
}

package statusservice

import (
	"sync"

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
}

// NewWSConnKeeper creates manager
func NewWSConnKeeper() *WSConnKeeper {
	res := &WSConnKeeper{}
	res.idConnectionMap = make(map[string]map[WsConn]struct{})
	res.connectionIDMap = make(map[WsConn]string)
	res.mapLock = &sync.Mutex{}
	return res
}

// HandleConnection loops until connection active and save connection with provided ID as key
func (kp *WSConnKeeper) HandleConnection(conn WsConn) error {
	defer kp.deleteConnection(conn)
	defer conn.Close()
	for {
		goapp.Log.Info().Msg("handleConnection")
		_, message, err := conn.ReadMessage()
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			break
		} else {
			kp.saveConnection(conn, string(message))
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
	goapp.Log.Info().Msgf("deleteConnection finish: %d", len(kp.connectionIDMap))
}

func (kp *WSConnKeeper) saveConnection(conn WsConn, id string) {
	goapp.Log.Info().Msg("saveConnection")
	kp.mapLock.Lock()
	defer kp.mapLock.Unlock()
	kp.deleteConnectionNoSync(conn)
	kp.connectionIDMap[conn] = id
	conns, found := kp.idConnectionMap[id]
	if !found {
		conns = map[WsConn]struct{}{}
		kp.idConnectionMap[id] = conns
	}
	conns[conn] = struct{}{}
	goapp.Log.Info().Msgf("saveConnection finish: %d", len(kp.connectionIDMap))
}

// GetConnections returns saved connections by provided id
func (kp *WSConnKeeper) GetConnections(id string) ([]WsConn, bool) {
	cm, found := kp.idConnectionMap[id]
	if found {
		res := []WsConn{}
		for cm := range cm {
			res = append(res, cm)
		}
		return res, true
	}
	return nil, false
}

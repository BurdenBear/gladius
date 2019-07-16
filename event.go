package gladius

import (
	"fmt"
	"sync"

	"github.com/deckarep/golang-set"
	"github.com/nntaoli-project/GoEx"
)

type EventType int

const (
	EVENT_STOP EventType = 0 + iota
	EVENT_REGISTER
	EVENT_UNREGISTER
	EVENT_ORDER
	EVENT_TRADE
	EVENT_ACCOUNT
	EVENT_TICKER
	EVENT_DEPTH
)

var eventTypeCursor = EVENT_DEPTH

var eventTypeNames = [...]string{
	"Stop",
	"Register",
	"Unregister",
	"Order",
	"Trade",
	"Account",
	"Ticker",
	"Depth",
}

var eventTypeNameMap = make(map[int]string)

func init() {
	for i, v := range eventTypeNames {
		eventTypeNameMap[i] = v
	}
}

func AddEventType(name string) (EventType, error) {
	exist := false
	for _, v := range eventTypeNameMap {
		if v == name {
			exist = true
			break
		}
	}
	if exist {
		return 0, fmt.Errorf("EventType (%s) has already exist", name)
	}
	eventTypeCursor = EventType(int(eventTypeCursor) + 1)
	eventTypeNameMap[int(eventTypeCursor)] = name
	return eventTypeCursor, nil
}

func (e EventType) String() string { return eventTypeNameMap[int(e)] }

type Event struct {
	Type EventType
	Data interface{}
}

func NewEvent(_type EventType, data interface{}) *Event {
	return &Event{_type, data}
}

type EventRegisterData struct {
	Type    EventType
	Handler *EventHandler
}

type EventHandler struct {
	Type    EventType
	Handler func(*Event)
}

func (handler *EventHandler) Process(event *Event) {
	handler.Handler(event)
}

func NewStopEventHandler(handler func()) *EventHandler {
	return &EventHandler{
		Type: EVENT_STOP,
		Handler: func(event *Event) {
			handler()
		},
	}
}

func NewOrderEventHandler(handler func(*Order)) *EventHandler {
	return &EventHandler{
		Type: EVENT_ORDER,
		Handler: func(event *Event) {
			data, ok := event.Data.(*Order)
			if ok {
				handler(data)
			} else {
				logger.Warningf("invaild data type of event: %+v", event)
			}
		},
	}
}

func NewTradeEventHandler(handler func(*goex.Trade)) *EventHandler {
	return &EventHandler{
		Type: EVENT_TRADE,
		Handler: func(event *Event) {
			data, ok := event.Data.(*goex.Trade)
			if ok {
				handler(data)
			} else {
				logger.Warningf("invaild data type of event: %+v", event)
			}
		},
	}
}

func NewAccountEventHandler(handler func(*goex.Account)) *EventHandler {
	return &EventHandler{
		Type: EVENT_ACCOUNT,
		Handler: func(event *Event) {
			data, ok := event.Data.(*goex.Account)
			if ok {
				handler(data)
			} else {
				logger.Warningf("invaild data type of event: %+v", event)
			}
		},
	}
}

func NewTickerEventHandler(handler func(*Ticker)) *EventHandler {
	return &EventHandler{
		Type: EVENT_TICKER,
		Handler: func(event *Event) {
			data, ok := event.Data.(*Ticker)
			if ok {
				handler(data)
			} else {
				logger.Warningf("invaild data type of event: %+v", event)
			}
		},
	}
}

func NewDepthEventHandler(handler func(*Depth)) *EventHandler {
	return &EventHandler{
		Type: EVENT_DEPTH,
		Handler: func(event *Event) {
			data, ok := event.Data.(*Depth)
			if ok {
				handler(data)
			} else {
				logger.Warningf("invaild data type of event: %+v", event)
			}
		},
	}
}

type EventEngine struct {
	queueSize int
	queue     chan *Event
	handlers  []mapset.Set
	isRunning bool
	rw        *sync.RWMutex
	sig       chan int
}

func NewEventEngine(queueSize int) *EventEngine {
	engine := &EventEngine{}
	engine.queueSize = queueSize
	engine.queue = make(chan *Event, queueSize)
	engine.handlers = make([]mapset.Set, len(eventTypeNames))
	handlersRegister := mapset.NewThreadUnsafeSet()
	handlersRegister.Add(&EventHandler{
		Type:    EVENT_REGISTER,
		Handler: engine.OnRegister,
	})
	engine.handlers[int(EVENT_REGISTER)] = handlersRegister
	engine.sig = make(chan int)
	engine.rw = &sync.RWMutex{}
	return engine
}

func (engine *EventEngine) GetQueueSize() int {
	return engine.queueSize
}

func (engine *EventEngine) OnRegister(event *Event) {
	data, ok := event.Data.(*EventRegisterData)
	if ok {
		if data.Handler.Type != data.Type {
			logger.Warning("register failed: inconsistent eventType")
		} else {
			if engine.handlers[data.Type] == nil {
				engine.handlers[data.Type] = mapset.NewThreadUnsafeSet()
			}
			engine.handlers[data.Type].Add(data.Handler)
		}
	} else {
		logger.Warningf("invaild data type of event: %+v", event)
	}
}

func (engine *EventEngine) Register(eventType EventType, handler *EventHandler) {
	engine.Process(&Event{
		Type: EVENT_REGISTER,
		Data: &EventRegisterData{
			Type:    eventType,
			Handler: handler,
		},
	})
}

func (engine *EventEngine) ProcessData(eventType EventType, data interface{}) {
	event := NewEvent(eventType, data)
	engine.Push(event)
}

func (engine *EventEngine) Process(event *Event) {
	handlers := engine.handlers[event.Type]
	if handlers != nil {
		it := handlers.Iterator()
		for handler := range it.C {
			handler.(*EventHandler).Process(event)
		}
	}
}

func (engine *EventEngine) IsRunning() bool {
	engine.rw.RLock()
	defer engine.rw.RUnlock()
	return engine.isRunning
}

func (engine *EventEngine) setIsRunning(isRunning bool) {
	engine.rw.Lock()
	defer engine.rw.Unlock()
	engine.isRunning = isRunning
}

func (engine *EventEngine) Push(event *Event) {
	if engine.IsRunning() {
		engine.queue <- event
	} else {
		logger.Debug("EventEngine has stopped, skip push event.")
	}
}

func (engine *EventEngine) run() {
	isStopReceived := false
	for engine.IsRunning() || !isStopReceived {
		event := <-engine.queue
		if event.Type == EVENT_STOP {
			isStopReceived = true
		}
		engine.Process(event)
	}
	engine.sig <- 0
}

func (engine *EventEngine) Start() {
	if !engine.IsRunning() {
		engine.setIsRunning(true)
		go engine.run()
	}
}

func (engine *EventEngine) Stop() {
	if engine.IsRunning() {
		engine.setIsRunning(false) // stop push event into queue, but will handle left event in queue before stop
		engine.queue <- NewEvent(EVENT_STOP, nil)
		<-engine.sig // wait run goroutine to exit
	}
}

package gladius

import (
	"sync"

	"github.com/deckarep/golang-set"
	"github.com/nntaoli-project/GoEx"
)

type EventType int

const (
	EVENT_STOP EventType = 0 + iota
	EVENT_ORDER
	EVENT_TRADE
	EVENT_ACCOUNT
	EVENT_TICKER
	EVENT_DEPTH
)

var eventTypes = [...]string{
	"Stop",
	"Order",
	"Trade",
	"Account",
	"Ticker",
	"Depth",
}

func (e EventType) String() string { return eventTypes[e] }

type Event struct {
	Type EventType
	Data interface{}
}

func NewEvent(_type EventType, data interface{}) Event {
	return Event{_type, data}
}

type EventHandler struct {
	Type    EventType
	handler func(Event)
}

func (handler *EventHandler) Process(event Event) {
	handler.handler(event)
}

func NewStopEventHandler(handler func()) *EventHandler {
	return &EventHandler{
		Type: EVENT_STOP,
		handler: func(event Event) {
			handler()
		},
	}
}

func NewOrderEventHandler(handler func(*Order)) *EventHandler {
	return &EventHandler{
		Type: EVENT_ORDER,
		handler: func(event Event) {
			data, ok := event.Data.(*Order)
			if ok {
				handler(data)
			} else {
				log.Warning("invaild data type of event")
			}
		},
	}
}

func NewTradeEventHandler(handler func(*goex.Trade)) *EventHandler {
	return &EventHandler{
		Type: EVENT_TRADE,
		handler: func(event Event) {
			data, ok := event.Data.(*goex.Trade)
			if ok {
				handler(data)
			} else {
				log.Warning("invaild data type of event")
			}
		},
	}
}

func NewAccountEventHandler(handler func(*goex.Account)) *EventHandler {
	return &EventHandler{
		Type: EVENT_ACCOUNT,
		handler: func(event Event) {
			data, ok := event.Data.(*goex.Account)
			if ok {
				handler(data)
			} else {
				log.Warning("invaild data type of event")
			}
		},
	}
}

func NewTickerEventHandler(handler func(*Ticker)) *EventHandler {
	return &EventHandler{
		Type: EVENT_TICKER,
		handler: func(event Event) {
			data, ok := event.Data.(*Ticker)
			if ok {
				handler(data)
			} else {
				log.Warning("invaild data type of event")
			}
		},
	}
}

func NewDepthEventHandler(handler func(*Depth)) *EventHandler {
	return &EventHandler{
		Type: EVENT_DEPTH,
		handler: func(event Event) {
			data, ok := event.Data.(*Depth)
			if ok {
				handler(data)
			} else {
				log.Warning("invaild data type of event")
			}
		},
	}
}

type EventEngine struct {
	queueSize  int
	queue      chan Event
	handlers   []mapset.Set
	isRunning  bool
	rw         *sync.RWMutex
	rwHandlers *sync.RWMutex
	sig        chan int
}

func NewEventEngine(queueSize int) *EventEngine {
	engine := &EventEngine{}
	engine.queueSize = queueSize
	engine.queue = make(chan Event, queueSize)
	engine.handlers = make([]mapset.Set, len(eventTypes))
	engine.sig = make(chan int)
	engine.rw = &sync.RWMutex{}
	engine.rwHandlers = &sync.RWMutex{}
	return engine
}

func (engine *EventEngine) Register(eventType EventType, handler *EventHandler) {
	engine.rwHandlers.Lock()
	defer engine.rwHandlers.Unlock()
	if handler.Type != eventType {
		log.Warning("register failed: inconsistent eventType")
	} else {
		if engine.handlers[eventType] == nil {
			engine.handlers[eventType] = mapset.NewThreadUnsafeSet()
		}
		engine.handlers[eventType].Add(handler)
	}
}

func (engine *EventEngine) ProcessData(eventType EventType, data interface{}) {
	event := NewEvent(eventType, data)
	engine.Push(event)
}

func (engine *EventEngine) Process(event Event) {
	engine.rwHandlers.RLock()
	defer engine.rwHandlers.RUnlock()
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

func (engine *EventEngine) Push(event Event) {
	if engine.IsRunning() {
		engine.queue <- event
	} else {
		log.Debug("EventEngine has stopped, skip push event.")
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

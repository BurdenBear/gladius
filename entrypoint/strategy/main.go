package main

import (
	"fmt"
	"log"

	. "github.com/BurdenBear/gladius"
	"github.com/BurdenBear/gladius/strategy"
)

type Strategy struct {
	*strategy.Strategy
}

func (stg *Strategy) OnDepth(depth *Depth) {
	log.Println(fmt.Sprintf("%s: %+v", depth.Contract.GetID(), depth))
}

func NewStrategy(name string, router *strategy.Router) strategy.IStrategy {
	return &Strategy{Strategy: strategy.NewStrategy(name, router)}
}

func main() {
	log.Println("call main")
}

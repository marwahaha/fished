package fished

import (
	"fmt"
	"sync"

	"github.com/knetic/govaluate"
)

// Engine ...
type Engine struct {
	Facts         map[string]interface{}
	wm            map[string]interface{}
	Rules         []Rule
	workingRules  map[string][]int
	RuleFunctions map[string]govaluate.ExpressionFunction
	Jobs          chan int
	Worker        int
	work          sync.WaitGroup
	wmLock        sync.Mutex
	planLock      sync.Mutex
}

// Rule ...
type Rule struct {
	Output string   `json:"output"`
	Input  []string `json:"input"`
	Rule   string   `json:"rule"`
	Value  string   `json:"value"`
}

// RuleRaw ...
type RuleRaw struct {
	Data []Rule `json:"data"`
}

var workerPoolSize = 10

// New ...
func New(worker int) *Engine {
	e := &Engine{
		Jobs:          make(chan int, worker*workerPoolSize),
		Worker:        worker,
		Facts:         make(map[string]interface{}),
		RuleFunctions: make(map[string]govaluate.ExpressionFunction),
	}
	return e
}

// Run ...
func (e *Engine) Run() interface{} {
	var wg sync.WaitGroup
	e.wm = make(map[string]interface{})
	for i, v := range e.Facts {
		e.wm[i] = v
	}

	e.workingRules = make(map[string][]int)
	for i, rule := range e.Rules {
		for _, input := range rule.Input {
			if e.workingRules[input] == nil {
				e.workingRules[input] = []int{i}
			} else {
				e.workingRules[input] = append(e.workingRules[input], i)
			}
		}
	}

	e.createAgenda()
	for i := 0; i < e.Worker; i++ {
		wg.Add(1)
		go e.worker(&wg)
	}
	e.watcher()
	wg.Wait()

	return e.wm["result_end"]
}

func (e *Engine) watcher() {
	e.work.Wait()
	close(e.Jobs)
}

func (e *Engine) worker(wg *sync.WaitGroup) {
	for job := range e.Jobs {
		e.eval(job)
		e.work.Done()
	}
	wg.Done()
}

// eval return true or false that will invoke need to update agenda or not.
func (e *Engine) eval(index int) {
	re, err := govaluate.NewEvaluableExpressionWithFunctions(e.Rules[index].Rule, make(map[string]govaluate.ExpressionFunction))
	if err != nil {
		panic(err)
	}
	valid, err := re.Evaluate(e.wm)
	if err != nil {
		panic(err)
	}

	if valid != nil && valid.(bool) {
		ve, err := govaluate.NewEvaluableExpressionWithFunctions(e.Rules[index].Value, e.RuleFunctions)
		if err == nil {
			res, _ := ve.Evaluate(nil)

			if e.Rules[index].Output != "" {
				e.wmLock.Lock()
				e.wm[e.Rules[index].Output] = res
				e.wmLock.Unlock()
				e.updateAgenda(e.Rules[index].Output)
			}
		}
	}
}

func (e *Engine) updateAgenda(input string) {
	e.planLock.Lock()
	defer e.planLock.Unlock()
	e.wmLock.Lock()
	defer e.wmLock.Unlock()

	rules := e.workingRules[input]
	for _, i := range rules {
		rule := e.Rules[i]
		validInput := 0
		for attribute := range e.wm {
			for _, input := range rule.Input {
				if input == attribute {
					validInput++
				}
			}
		}
		if validInput == len(rule.Input) && validInput != 0 {
			fmt.Println("Output target:", rule.Output, "Added")
			e.work.Add(1)
			e.Jobs <- i
		}
	}
	delete(e.workingRules, input)
}

func (e *Engine) createAgenda() {
	for attribute := range e.wm {
		e.updateAgenda(attribute)
	}
}

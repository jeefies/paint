package main

import (
	"bytes"
	"fmt"
	"io"
	draw "jeefy/drawer"
	"log"
	"os"
	"time"
)

type Task struct {
	conf io.Reader
	draw *draw.ImageDrawer
}

type TaskRunner struct {
	api   *draw.Api
	tasks []*Task
}

func NewTaskRunner(api *draw.Api) *TaskRunner {
	return &TaskRunner{api, make([]*Task, 0, 5)}
}

func (run *TaskRunner) NewTask(conf io.Reader) *Task {
	return &Task{conf, draw.NewDrawer(run.api)}
}

func (run *TaskRunner) ReadTasks(f io.Reader) {
	var n int
	fmt.Fscanln(f, &n)
	for i := 0; i < n; i++ {
		var confFile string
		fmt.Fscanln(f, &confFile)
		fp, err := os.Open(confFile)
		if err != nil {
			log.Println("Proc file", confFile, "failed: ", err)
			continue
		}
		defer fp.Close()

		bs, _ := io.ReadAll(fp)
		log.Println("Read Task: ", string(bs))
		run.tasks = append(run.tasks, run.NewTask(bytes.NewReader(bs)))
	}
}

func (run *TaskRunner) Mainloop() {
	for k := range run.tasks {
		task := run.tasks[k]
		readConfig(task.conf, task.draw)
		log.Println("Start Task: ", k)
		task.draw.Start()

		for i := 1; i <= 20; i++ {
			log.Println("Wait ", i, "seconds")
			time.Sleep(time.Second)
		}

		for task.draw.WorkStatus() != 0 {
			time.Sleep(10 * time.Second)
		}

		log.Println("Task: ", k, "...Done !!!")
	}
}

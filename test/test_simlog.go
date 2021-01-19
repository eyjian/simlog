// Writed by yijian on 2020/12/25
package main

import (
    "flag"
    "fmt"
    "os"
    "sync"
)
import (
    "github.com/eyjian/simlog"
)

var (
    help             = flag.Bool("h", false, "Display a help message and exit.")
    enableAsyncWrite = flag.Bool("eaw", true, "Enable write log to file asynchronously.")
    enableLineFeed   = flag.Bool("elf", true, "Add '\\n' at the end of the log line automatically.")
)

func main() {
    var wg sync.WaitGroup
    var simlogger simlog.SimLogger

    flag.Parse()
    if *help {
        flag.Usage()
        os.Exit(1)
    }
    if !simlogger.Init(
        simlog.EnableAsyncWrite(*enableAsyncWrite),
        simlog.WithSubPrefix("PREFIX"),
        simlog.WithSubSuffix("SUFFIX"),
        simlog.WithTag("TEST"),
        simlog.EnableAsyncWrite(*enableAsyncWrite),
        simlog.EnableLineFeed(*enableLineFeed),
        simlog.WithLogObserver(logObserver)) {
        fmt.Printf("Init simlog failed\n")
        os.Exit(1)
    }

    simlogger.Info("Info level: ", simlogger.EnabledLineFeed(), "\n") // 1

    simlogger.EnableLogCaller(true)
    simlogger.Infof("Log caller enabled\n") // 2

    simlogger.EnableLineFeed(true)
    simlogger.Infof("Linefeed enabled") // 3

    simlogger.EnableRawLog(true, false)
    simlogger.Raw("raw log") // 4

    simlogger.EnableRawLog(true, true)
    simlogger.Raw("raw log with time") // 5

    for i:=0; i<5; i++ {
        wg.Add(1)
        go func (i int) {
            simlogger.Infof("%d", i)
            wg.Done()
        }(i)
    } // 10
    wg.Wait()

    simlogger.Infof("Exit now") // 11
    simlogger.Close()
}

func logObserver(logLevel simlog.LogLevel, logHeader string, logBody string) {
    fmt.Printf("[OBSERVED][%s]%s%s\n", simlog.GetLogLevelName(logLevel), logHeader, logBody)
}

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
    numLogs          = flag.Int("logs", 5, "Number of logs written by each coroutine.")
    numCoroutines    = flag.Int("coroutines", 2, "Number of coroutines.")
    fileSize         = flag.Int("size", 0, "Size of log file.")
    lockOSThread     = flag.Bool("lockosthread", false, "Lock OS thread.")
    observer         = flag.Bool("observer", false, "Enable log observer.")
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
        simlog.EnableLockOSThread(*lockOSThread),
        simlog.EnableAsyncWrite(*enableAsyncWrite),
        simlog.EnableLineFeed(*enableLineFeed),
        simlog.WithLogObserver(logObserver)) {
        fmt.Printf("Init simlog failed\n")
        os.Exit(1)
    }

    if *fileSize > 0 {
        simlogger.SetLogFileSize(int64(*fileSize))
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

    for i:=0; i<*numCoroutines; i++ {
        wg.Add(1)
        go func (i int) {
            for j:=0; j<*numLogs; j++ {
                simlogger.Infof("%030d => %030d", i, j)
            }
            wg.Done()
        }(i)
    }
    if *numCoroutines > 0 {
        wg.Wait()
    }

    simlogger.Infof("Exit now")
    simlogger.Close()
}

func logObserver(logLevel simlog.LogLevel, logHeader string, logBody string) {
    if *observer {
        fmt.Printf("[OBSERVED][%s]%s%s\n", simlog.GetLogLevelName(logLevel), logHeader, logBody)
    }
}

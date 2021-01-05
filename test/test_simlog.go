// Writed by yijian on 2020/12/25
package main

import (
    "fmt"
    "os"
)
import (
    "github.com/eyjian/simlog"
)

func main() {
    var simlogger simlog.SimLogger

    simlogger.SetSubPrefix("PREFIX")
    simlogger.SetSubSuffix("SUFFIX")
    if !simlogger.Init() {
        fmt.Printf("Init simlog failed\n")
        os.Exit(1)
    }

    simlogger.Infof("Info level\n")
    simlogger.SetTag("TEST")
    simlogger.Infof("Tag is set to TEST\n")

    simlogger.EnableLogCaller(true)
    simlogger.Infof("Log caller enabled\n")

    simlogger.EnableLineFeed(true)
    simlogger.Infof("Linefeed enabled")

    simlogger.EnableRawLog(true, false)
    simlogger.Raw("raw log")

    simlogger.EnableRawLog(true, true)
    simlogger.Raw("raw log with time")

    simlogger.SetLogObserver(logObserver)
    simlogger.Infof("Exit now")
}

func logObserver(logLevel simlog.LogLevel, logHeader string, logBody string) {
    fmt.Printf("[OBSERVED][%s]%s%s\n", simlog.GetLogLevelName(logLevel), logHeader, logBody)
}

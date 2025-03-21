// *
// * Author       :loyd
// * Date         :2024-11-10 21:49:51
// * LastEditors  :loyd
// * LastEditTime :2024-11-11 11:41:20
// * Description  :
// *
// *

package logger

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var log = logrus.New()
var once sync.Once
var Logger *logrus.Logger

type CustomFormatter struct{}

func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := time.Now().Format(time.RFC3339)
	log := fmt.Sprintf("%s [%s] %s: %s\n", timestamp, entry.Level, entry.Data, entry.Message)
	return []byte(log), nil
}

func init() {
	once.Do(func() {
		// set custom formatter
		log.SetFormatter(&CustomFormatter{})

		// set std output
		log.SetOutput(os.Stdout)
	})
}

func GetLogger() *logrus.Logger {
	return log
}

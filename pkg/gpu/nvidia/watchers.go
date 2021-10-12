package nvidia

import (
	"os"
	"os/signal"

	"github.com/fsnotify/fsnotify"
)

func newFSWatcher(files ...string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	//fmt.Println("7")
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		//fmt.Println("9")
		err = watcher.Add(f)
		if err != nil {
			watcher.Close()
			return nil, err
		}
	}

	return watcher, nil
}

func newOSWatcher(sigs ...os.Signal) chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	//fmt.Println("8")
	signal.Notify(sigChan, sigs...)

	return sigChan
}

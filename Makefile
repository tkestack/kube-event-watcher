tke-event-watcher: fmt vet
	go build -o tke-event-watcher cmd/tke_event_watcher.go
fmt:
	go fmt ./cmd/...
vet:
	go vet ./cmd/...

docker:
	hack/build-watcher.sh

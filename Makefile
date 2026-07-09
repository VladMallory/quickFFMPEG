run: 
	go build -ldflags "-X 'main.BuildTime=$$(date -u +%Y-%m-%dT%H:%M:%SZ)'"

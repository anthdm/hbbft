deps:
	@dep ensure

test: 
	@go test ./... -v --cover
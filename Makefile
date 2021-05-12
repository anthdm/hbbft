test:
	@go test ./... --cover

test-randomized:
	go test --count 10000 --run TestBBARandomized
	go test --count 10000 --run TestRBCRandomized
	go test --count 10000 --run TestACSRandomized

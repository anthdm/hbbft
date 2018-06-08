# Simulation

### How to run

If in root folder of the hbbft project:
```
go run simulation/main.go 
```

> Give the simulation couple seconds to come up to speed. In the first epochs the transaction buffer will be empty or very small. 

### Tuning parameters

The following parameters can be tuned
- lenNodes (default 4)
- batchSize (default 500)
- numCores (default 4)
- transaction verification delay (default 2ms)


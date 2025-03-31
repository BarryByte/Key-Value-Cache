
# Optimized Go In-Memory Key-Value Cache

## Overview

**In Simple Terms:**

Imagine a super-fast, temporary notepad built right into your computer's memory. This program lets you quickly jot down (PUT) pieces of information using a unique label (the "key") and look them up again (GET) instantly using the same label. It's designed to be very efficient and careful with memory, especially on smaller computers or cloud servers, automatically tidying up old notes when it gets full.

**Technically Speaking:**

This project provides a high-performance, concurrent, in-memory key-value cache server implemented in Go. It is optimized for scenarios requiring low latency access to frequently used data while operating within strict memory constraints (e.g., AWS t3.small instances). Key optimizations include:

1.  **LRU (Least Recently Used) Eviction:** Ensures the cache size stays within a defined limit by automatically removing the least recently accessed items when capacity is reached.
2.  **Sharding (Partitioning):** Divides the cache data and associated locks across multiple independent shards, significantly reducing lock contention and improving throughput on multi-core processors.

## Key Features

* **Fast In-Memory Access:** Provides low-latency read and write operations.
* **Bounded Memory Usage:** Implements LRU eviction to prevent uncontrolled memory growth.
* **High Concurrency:** Utilizes sharding with per-shard mutexes for improved parallelism.
* **Simple HTTP API:** Offers `/get`, `/put`, and `/health` endpoints.
* **Configurable:** Shard count and capacity per shard can be tuned via constants.

## Design Choices (Why This Approach?)

The goal was to create a cache suitable for resource-constrained environments (like AWS t3.small: 2-core, 2GB RAM) where an external cache (like Redis/Memcached) might be overkill or introduce unwanted network latency/complexity.

* **Why not the original simple map with one lock?**
    * **Memory:** The original approach had no memory limit (`MaxCapacity` was unused), risking Out-Of-Memory (OOM) crashes.
    * **Concurrency:** A single global lock (`sync.RWMutex`) becomes a bottleneck under high concurrent load, especially with mixed reads and writes, limiting the ability to utilize multiple CPU cores effectively.

* **Why LRU Eviction?**
    * LRU is a standard cache eviction policy that provides a good balance between implementation complexity and effectiveness. It ensures that the most *relevant* data (recently accessed) is retained while staying within strict memory bounds.

* **Why Sharding?**
    * Sharding directly addresses the single-lock bottleneck. By splitting the data and locks across, say, 64 shards, the probability of two concurrent requests needing the *same* lock is significantly reduced. This allows multiple cores to process requests in parallel much more effectively.

## Potential Issues & Observations During Testing

**Observation:**

When load testing this cache (e.g., using Locust) immediately after starting the server, you might observe a high initial failure rate (e.g., 40-50%) that gradually decreases to near zero over a period of minutes (2-8 minutes in observed tests). If this process stopped and again started it will result to 0 % failure because this time our cache system is **warmed-up** .

> Initial Test result
![Initial Test result](https://github.com/user-attachments/assets/0f6b2de9-fdd4-4b7d-ba7e-f9119976f178)

> After Warm up : 0 % Failure ( see after 11:44:57 ) 
![After Warm up](https://github.com/user-attachments/assets/c3f64388-f95f-47a4-9349-4224ca7841e5)


**Likely Explanation:**

This behavior is generally **not indicative of a bug** in the cache logic itself but rather an interaction between the **test methodology and the nature of a cache**:

1.  **Cache Starts Empty:** The server initializes with no stored key-value pairs.
2.  **Initial Cache Misses:** Load tests often start sending `GET` requests immediately. Requests for keys not yet present in the cache result in a "cache miss".
3.  **HTTP 404 Response:** The cache server correctly responds with `HTTP 404 Not Found` for these misses.
4.  **Load Tester Interpretation:** Tools like Locust typically treat all `4xx` (Client Error) and `5xx` (Server Error) responses as "failures" by default.
5.  **Cache Warm-up:** As the load test simultaneously sends `PUT` requests, the cache begins to populate ("warm-up").
6.  **Decreasing Failure Rate:** With more keys added, subsequent `GET` requests have a higher probability of being cache hits (returning `HTTP 200 OK`). Since `200 OK` is a success, the reported failure rate decreases as the cache hit rate increases.

Essentially, the initial "failures" reported by the testing tool are often just the expected `404 Not Found` responses during the cache's warm-up phase. To confirm this, examine the specific failure details in your load testing tool – they should primarily show `404` errors for `/get` requests during the initial phase.

## Setup and Running

**Prerequisites:**

* Go (Version 1.24)

**Build:**

```bash
docker build -t kv-go-cache:latest .
```

**Run:**

```bash
docker run -p 7171:7171 kv-go-cache:latest

```
**Test: paste this on postman**

```bash
curl -X POST "http://localhost:7171/put" -H "Content-Type: application/json" -d '{"key": "name", "value": "Alice"}'

curl -X GET "http://localhost:7171/get?key=name"

```

**Load Test:**

```bash
locust -f locustfile.py --host=http://localhost:7171
```


![Build and run](https://github.com/user-attachments/assets/77ad7298-834a-4fe9-86b0-eac229b1438f)





## License
This project is licensed under the MIT License.

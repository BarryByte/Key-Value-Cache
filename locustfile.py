# import string
# import random
# import uuid
# from locust import FastHttpUser, task, constant

# # Configuration
# KEY_POOL_SIZE = 10_000  # Number of keys shared across all users
# VALUE_LENGTH = 256
# PUT_RATIO = 0.5  # 50% PUT, 50% GET requests

# class CacheUser(FastHttpUser):
#     wait_time = constant(0)  # No wait time for maximum throughput

#     # Shared data across users
#     key_pool = [str(uuid.uuid4()) for _ in range(KEY_POOL_SIZE)]
#     value_pool = [''.join(random.choices(string.printable, k=VALUE_LENGTH)) for _ in range(KEY_POOL_SIZE)]

#     @task
#     def mixed_load(self):
#         """Send a mix of PUT and GET requests based on PUT_RATIO."""
#         if random.random() < PUT_RATIO:
#             self.put_request()
#         else:
#             self.get_request()

#     def put_request(self):
#         """Send a PUT request to store a key-value pair."""
#         key = random.choice(self.key_pool)
#         value = random.choice(self.value_pool)
#         self.client.post("/put", json={"key": key, "value": value}, name="/put")

#     def get_request(self):
#         """Send a GET request to retrieve a key's value."""
#         key = random.choice(self.key_pool)
#         self.client.get(f"/get?key={key}", name="/get")


import string

from locust import HttpUser, task, FastHttpUser, constant
import random
import uuid
from functools import lru_cache


# Configuration
KEY_POOL_SIZE = 10_000  # Shared across all users
VALUE_LENGTH = 256
PUT_RATIO = 0.5  # 50% PUT requests


class CacheUser(FastHttpUser):
    # Disable wait time for max throughput
    wait_time = constant(0)

    # Shared across all users using class variables
    key_pool = [str(uuid.uuid4()) for _ in range(KEY_POOL_SIZE)]
    value_pool = [''.join(random.choices(string.printable, k=VALUE_LENGTH)) for _ in range(KEY_POOL_SIZE)]

    @task
    def mixed_load(self):
        """50/50 GET/PUT ratio with cache-friendly keys"""
        if random.random() < PUT_RATIO:
            self.put_request()
        else:
            self.get_request()

    def put_request(self):
        key = random.choice(self.key_pool)
        value = random.choice(self.value_pool)
        self.client.post(
            "/put",
            json={"key": key, "value": value},
            name="/put"
        )

    def get_request(self):
        key = random.choice(self.key_pool)  # Higher cache hit rate
        self.client.get(
            f"/get?key={key}",
            name="/get"
        )
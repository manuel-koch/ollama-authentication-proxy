# Using ollama-authentication-proxy on runpod

A short introduction to use this image to run a serverless / load-balanced
instance of ollama on [runpod](https://docs.runpod.io/serverless/load-balancing/overview).

Create a new runpod __endpoint__ with the following settings:

- Choose __Load Balanced__ style of serverless endpoint.

- Choose a __GPU Configuration__ for the worker that fits the targeted ollama models you want to run.

- Configure __Idle Timeout__ to save money when nobody is using the ollama instance.

- Configure __Environment Variables__ like

| Name              | Value                        | Hint                                                                           |
|-------------------|------------------------------|--------------------------------------------------------------------------------|
| OLLAMA_HOST       | 127.0.0.1:11434              | Ollama will bind to this address                                               |
| OLLAMA_MODELS     | /runpod-volume/ollama-models | (Worker uses mounted volume) Ollama will download models to this directory.    |
| PORT              | 80                           | The HTTP port served by ollama-authentication-proxy                            |
| PORT_HEALTH       | 80                           | The HTTP port for health-check ( /ping ) served by ollama-authentication-proxy |
| PRELOAD_MODEL_A   | gemma3:27b                   | An ollama model you want to preload on startup of worker                       |
| PRELOAD_MODEL_C   | gpt-oss:20b                  | Another ollama model to preload                                                |
| PRELOAD_MODEL_... | ...                          | Another ollama model to preload                                                |

- Under __Docker Configuration__
  - Select the docker image, use latest tag published, see [docker hub](https://hub.docker.com/repository/docker/brilliantcreator/ollama-authentication-proxy).
  - Configure "Container Disk" as necessary ( can be small if you are using a shared volume ).
  - Expose HTTP port(s) according to your `PORT` & `PORT_HEALTH` configuration above.

- Under __Advanced__
  - You may choose to create a volume that can be bound to a worker,
    to share downloaded ollama models between the running worker instances.



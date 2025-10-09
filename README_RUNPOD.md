# Using ollama-authentication-proxy on runpod

A short introduction to use this image to run a serverless / load-balanced
instance of ollama on [runpod](https://docs.runpod.io/serverless/load-balancing/overview).

Create a new runpod __endpoint__ with the following settings:

- Choose __Load Balanced__ style of serverless endpoint.

- Choose a __GPU Configuration__ for the worker that fits the targeted ollama models you want to run.

- Configure __Max Workers__ to a reasonable limit.
  Don't run just one worker ! It can happen that the one worker becomes `throttled` and endpoint is not reachable.
  Just configure some greater __Max Workers__ ( e.g. two ) and use a higher __Request Count__ under section __Advanced__,
  so that just one worker is handling all the traffic.
  Configuring more than one worker seems to spawn actually more workers than needed, but since they are idle,
  the won't generate costs - only those workers that actually handle traffic generate costs !

- Configure __Idle Timeout__ to save money when nobody is using the ollama instance.

- Configure __Environment Variables__ like

| Name                               | Value                        | Hint                                                                           |
| ---------------------------------- | ---------------------------- | ------------------------------------------------------------------------------ |
| OLLAMA_HOST                        | 127.0.0.1:11434              | Ollama will bind to this address                                               |
| OLLAMA_MODELS                      | /runpod-volume/ollama-models | (Worker uses mounted volume) Ollama will download models to this directory.    |
| PORT                               | 80                           | The HTTP port served by ollama-authentication-proxy                            |
| PORT_HEALTH                        | 80                           | The HTTP port for health-check ( /ping ) served by ollama-authentication-proxy |
| PRELOAD_MODEL_A                    | gemma3:27b                   | An ollama model you want to preload on startup of worker                       |
| PRELOAD_MODEL_C                    | gpt-oss:20b                  | Another ollama model to preload                                                |
| PRELOAD_MODEL_...                  | ...                          | Another ollama model to preload                                                |
| AUTHORIZATION_LOG_LEVEL            | INFO                         |                                                                                |
| AUTHORIZATION_LOG_JSON             | true                         | log in JSON formated                                                           |
| USER_MODEL_METRICS_WEBHOOK_URL     | http://somewhere.com/webhook | Sent user model metrics to given webhook                                       |
| USER_MODEL_METRICS_WEBHOOK_API_KEY | <API-KEY>                    | Use given api-key to authorize webhook requests                                |

- Under __Docker Configuration__
  - Select the docker image, use latest tag published, see [docker hub](https://hub.docker.com/repository/docker/brilliantcreator/ollama-authentication-proxy).
  - Configure "Container Disk" as necessary ( can be small if you are using a shared volume ).
  - Expose HTTP port(s) according to your `PORT` & `PORT_HEALTH` configuration above.

- Under __Advanced__
  - You may choose to create a volume that can be bound to a worker,
    to share downloaded ollama models between the running worker instances.

## Hints

Following GPU types don't seem to be usable by ollama ( interferences are only running on CPU despite an attached GPU !? ):

- RTX A6000

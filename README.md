# Custom docker image to run `ollama` behind authentication proxy

Image for amd64/arm64 available at [docker hub](https://hub.docker.com/repository/docker/brilliantcreator/ollama-authentication-proxy).

Based on the original [ollama](https://hub.docker.com/r/ollama/ollama) docker image.

Using custom tool `ollama-authentication-proxy` to authenticate
incoming requests and proxy them to `ollama`.

Requests must provide header `Authorization: Bearer <APIKEY>` to access the `ollama` API.

Multiple API-Keys can be provided via environment variables like
- AUTHORIZATION_APIKEY=foo
- AUTHORIZATION_APIKEY_1=hello-world
- AUTHORIZATION_APIKEY_2=my-private-api-key

The container will use the following ports:
- 18434: Tool `ollama-authentication-proxy` to validate authorization and proxy requests to ollama
- 11434: Ollama

```mermaid
sequenceDiagram
    actor user
    Note over user,ollama-authentication-proxy: Access ollama API using header<br>"Authorization: Bearer <APIKEY>"
    user ->> ollama-authentication-proxy: GET /api/tags
    Note over ollama-authentication-proxy: authorize with header<br>"Authorization: Bearer <APIKEY>"
    Note over ollama-authentication-proxy, ollama: Forward the request
    ollama-authentication-proxy ->>+ ollama: GET /api/tags
    ollama ->>- ollama-authentication-proxy: response
    Note over ollama-authentication-proxy, user: Reply with data from ollama
    ollama-authentication-proxy ->> user: response
```
# Custom docker image to run `ollama` behind authentication proxy

Image available at [docker hub](https://hub.docker.com/repository/docker/brilliantcreator/ollama-authentication-proxy).

Based on the original ollama docker image.

Using custom tool `authorization-bearer` to authenticate incoming requests for ollama
via a NGINX hook.
Request must provide header `Authorization Bearer <APIKEY>` to access the ollama API.

Multiple API-Keys can be provided via environment variables like
- AUTHORIZATION_APIKEY=foo
- AUTHORIZATION_APIKEY_1=hello-world
- AUTHORIZATION_APIKEY_2=my-private-api-key

The container will use the following ports:
- 18434: Nginx reverse-proxy to provide authenticated endpoints for ollama
- 8080: Tool `authorization-bearer` to validate authorization
- 11434: Ollama

```mermaid
sequenceDiagram
    actor user
    Note over user,nginx: Access ollama API using<br>"Authorization Bearer <APIKEY>"
    user ->> nginx: GET /api/tags
    Note over nginx, authorization-bearer: validate with<br>"Authorization Bearer <APIKEY>"
    nginx ->>+ authorization-bearer: GET /auth
    authorization-bearer ->>- nginx: OK
    Note over nginx, ollama: Forward the request
    nginx ->>+ ollama: GET /api/tags
    ollama ->>- nginx: response
    Note over nginx, user: Reply with data from ollama
    nginx ->> user: response
```
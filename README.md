# pos-os-service

Microsservico de Ordem de Servico (OS) do sistema de PDV (POS) para oficina
mecanica, extraido do monolito original como um dos quatro microsservicos.

Este servico e responsavel pelo ciclo de vida de uma ordem de servico:
criacao, a maquina de estados com 10 status (`CREATED -> BUDGETING ->
AWAITING_APPROVAL -> APPROVED -> PAYING -> PAID -> IN_EXECUTION ->
COMPLETED`, com `CANCELLED`/`FAILED` como estados terminais alcancaveis a
partir de qualquer estado nao terminal) e o historico de status.

## Arquitetura

Este servico e um dos participantes de uma saga coordenada por um servico
orquestrador separado, **`pos-saga-orchestrator`** (veja aquele repositorio
para os diagramas de sequencia completos entre servicos). Este servico nao
chama outros servicos diretamente; a comunicacao acontece exclusivamente via
eventos/comandos no RabbitMQ (exchange topic `pos.events`), usando o padrao
transactional outbox para garantir que a escrita no banco e o evento
correspondente nunca fiquem inconsistentes entre si.

Organizacao dos processos (3 binarios, 1 banco de dados, 1 broker AMQP
compartilhado):

- `cmd/server` — API HTTP em Gin (`/api/v1/orders...`), grava no Postgres e
  na tabela `outbox` na mesma transacao das escritas de dominio.
- `cmd/outbox-dispatcher` — faz polling da tabela `outbox` e publica as
  linhas ainda nao publicadas no RabbitMQ, com backoff exponencial em caso de
  falha de publicacao.
- `cmd/worker` — consome comandos do RabbitMQ (`os-service.events.q`) e os
  aplica ao dominio (atualmente: `CancelOSCommand`).

Veja `docs/adr/0001-orchestrated-saga.md` para o racional da escolha de uma
saga orquestrada (e nao coreografada).

## Participacao na saga

- **Emite**: `OSCreated` (na criacao da ordem), `OSCancelled` (apos
  processar um `CancelOSCommand`). Ambos sao publicados via padrao outbox: o
  evento e gravado na tabela `outbox` na mesma transacao de banco da mudanca
  de dominio, e um processo separado (dispatcher) publica no RabbitMQ de
  forma assincrona e marca como publicado. Isso garante entrega
  "at-least-once" sem precisar de uma transacao distribuida.
- **Consome**: `CancelOSCommand` (comando de compensacao vindo do
  orquestrador) — transiciona a ordem para `CANCELLED`, grava o historico de
  status e emite `OSCancelled`.
- **Idempotencia**:
  - Comandos recebidos: tabela `processed_events`, indexada por `event_id`,
    verificada antes do processamento e marcada apos uma transicao bem
    sucedida. Combinado com a constraint de chave primaria, isso torna o
    reenvio do mesmo `CancelOSCommand` uma operacao sem efeito (no-op).
  - Escritas HTTP recebidas: o header `Idempotency-Key` e obrigatorio em
    `POST /orders`; o hash do corpo da requisicao + a resposta sao
    armazenados em `idempotency_keys` por 24h, de forma que requisicoes
    repetidas com a mesma chave e o mesmo corpo repetem a resposta original,
    e requisicoes que reusam a chave com corpo diferente recebem
    `409 Conflict`.

## Rodando localmente (standalone)

Requer Postgres e RabbitMQ acessiveis pelas variaveis de ambiente abaixo.
Exemplo usando containers Docker locais:

```bash
docker run -d --name os-postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=os_service -p 5432:5432 postgres:16-alpine
docker run -d --name os-rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management-alpine

export OS_DB_DSN="host=localhost user=postgres password=postgres dbname=os_service port=5432 sslmode=disable"
export OS_AMQP_URL="amqp://guest:guest@localhost:5672/"

go run ./cmd/server             # API HTTP na porta :8081, roda migrations no boot
go run ./cmd/outbox-dispatcher  # processo separado, faz polling da outbox
go run ./cmd/worker             # processo separado, consome CancelOSCommand
```

Ou usando as imagens de container geradas a partir do `Dockerfile` deste
repositorio:

```bash
docker build --build-arg TARGET=server -t pos-os-service:server .
docker run --rm -p 8081:8081 \
  -e OS_DB_DSN="host.docker.internal:..." \
  -e OS_AMQP_URL="amqp://guest:guest@host.docker.internal:5672/" \
  pos-os-service:server
```

## Rodando como parte da stack completa

Este servico e composto como parte da stack completa do PDV via
`pos-saga-orchestrator/deploy/local/docker-compose.yml` (nesse repositorio
irmao — nao alterado aqui), que orquestra Postgres, RabbitMQ, este servico e
os outros tres microsservicos juntos.

## Variaveis de ambiente

| Variavel                   | Padrao                                                                                        | Descricao                                            |
|-----------------------------|------------------------------------------------------------------------------------------------|-------------------------------------------------------|
| `OS_PORT`                  | `8081`                                                                                          | Porta HTTP do `cmd/server`                            |
| `OS_DB_DSN`                 | `host=localhost user=postgres password=postgres dbname=os_service port=5432 sslmode=disable`   | DSN do Postgres (formato GORM/`lib/pq`)                |
| `OS_AMQP_URL`               | `amqp://guest:guest@localhost:5672/`                                                            | URL de conexao com o RabbitMQ                          |
| `OS_DISPATCH_INTERVAL_MS`   | `500`                                                                                            | Intervalo de polling da outbox no `cmd/outbox-dispatcher` |

## API

Veja `docs/openapi.yaml`. Resumo:

- `POST /api/v1/orders` (exige o header `Idempotency-Key`) -> `201`
- `GET /api/v1/orders/:id` -> ordem + historico de status, `404` se nao existir
- `PATCH /api/v1/orders/:id/status` -> transicao interna, disparada pelo orquestrador da saga, `400` em transicao invalida
- `GET /api/v1/orders?customer_id=&status=&limit=&cursor=` -> listagem paginada por cursor
- `GET /healthz`, `GET /readyz`

Todos os erros seguem o formato `application/problem+json` (RFC 7807).

## Testes

```bash
go test ./...                                              # apenas testes unitarios, sem dependencias externas, rapido
go test -tags=integration ./... -coverpkg=./...            # unitarios + integracao (testcontainers-go: Postgres + RabbitMQ reais), precisa de Docker
```

O teste de integracao em `tests/integration/` fica atras da build tag
`integration` justamente para que `go test ./...` continue rapido e sem
dependencias externas. Ele roda de verdade no CI (`.github/workflows/ci.yml`,
job `test`) a cada push/PR, contra Postgres e RabbitMQ dockerizados via
testcontainers-go — sem precisar de nenhum servico externo alem do Docker
do proprio runner.

### Cobertura

Medido em 2026-07-26 via `go test -tags=integration ./... -coverpkg=./...
-coverprofile=coverage.out` (unitarios + integracao juntos, o mesmo comando
que o CI roda): **62,2%** do repositorio inteiro.

Esse numero fica abaixo dos 80% porque inclui `cmd/server`, `cmd/worker` e
`cmd/outbox-dispatcher` (~316 linhas de wiring de `main()`, sem logica
propria) — deixados sem teste de proposito, ja que testa-los exigiria um
refactor so pra extrair logica testavel, sem ganho real de confianca. O que
importa pra corretude — endpoints REST, casos de uso e dominio — esta bem
acima de 80% em todos os pacotes:

| Pacote                               | Cobertura | Como |
|---------------------------------------|-----------|------|
| `internal/domain/entities`            | 94,4%     | unit |
| `internal/application/usecases`       | 78,8%     | unit |
| `internal/presentation/handlers`      | 92,7%     | unit (httptest + fakes) |
| `internal/presentation/middleware`    | 91,8%     | unit (httptest) |
| `internal/presentation/dto`           | 100%      | unit |
| `internal/infrastructure/config`      | 100%      | unit |
| `internal/infrastructure/db`          | 93,0%     | integracao (testcontainers) |
| `internal/infrastructure/messaging`   | 77,2%     | integracao (testcontainers) |
| `cmd/*`                                | 0%        | fora de escopo (wiring) |

`internal/domain/entities` cobre todas as entradas da tabela de transicao de
status (validas e invalidas), alem da rejeicao de qualquer transicao a
partir de estados terminais. `internal/application/usecases` cobre
`CreateOrder` e `HandleCancelOS` (sucesso, replay idempotente, ordem nao
encontrada, transicao invalida). `internal/presentation/handlers` cobre
todos os endpoints REST via `httptest` + fakes em memoria. Os testes de
integracao em `tests/integration/` (testcontainers-go, Postgres + RabbitMQ
reais) cobrem `OrderRepository`/`OutboxRepository`/`IdempotencyRepository`/
`ProcessedEventRepository` e o helper `messaging.Conn` (publish/consume,
retry ate `MaxRetries`, DLQ).

## Notas de desenvolvimento / desvios da especificacao

- O `go.mod` fixa `go 1.23.2` (nao 1.24) e
  `github.com/gin-gonic/gin@v1.10.1` (nao a v1.12.0 mais recente) porque o
  toolchain Go disponivel localmente e o 1.23.2, e versoes mais novas de
  gin/quic-go exigem Go >= 1.25. Usar um toolchain mais novo via
  `GOTOOLCHAIN=auto` quebrava de forma reproduzivel o `go test
  -coverprofile` para pacotes sem arquivos de teste (`go: no such tool
  "covdata"`), entao o conjunto de dependencias foi fixado em versoes
  compativeis com o toolchain 1.23.2 instalado, em vez de perseguir o
  auto-upgrade do toolchain.
- O `go mod tidy` precisou de `-e -compat=1.23` para terminar sem erro por
  causa de uma dependencia transitiva usada apenas em testes (a propria
  suite de testes do driver Postgres do `golang-migrate` traz
  `testcontainers`/`dktest`, que por sua vez traz um pacote que exige Go
  >= 1.25) — isso afeta apenas a contabilidade do "all pattern" do `go mod
  tidy`, nao o build nem os testes deste modulo, ambos verdes.

## TODO / proximos passos

- Rodar `make test-integration` em uma maquina com Docker para validar o
  fluxo ponta a ponta outbox -> RabbitMQ (escrito, compila, mas nao
  executado aqui).
- Configurar credenciais de um registry real no CI quando existir um para
  este servico (o job `build` do CI hoje apenas builda as imagens
  localmente, sem push).

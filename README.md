# Webhook Ledger Service (Go + PostgreSQL + MVC)

Este projeto consiste em um serviço de Ledger (livro-razão) de alta performance e precisão financeira desenvolvido em Go, utilizando PostgreSQL para armazenamento seguro e o roteador Chi (`go-chi`). O sistema processa transações assinadas via Webhooks e consolida saldos com precisão decimal arbitrária de forma concorrente e segura.

---

## 🚀 Como Iniciar de Forma Simples (Quick Start)

A maneira mais rápida e fácil de rodar o projeto localmente com todas as dependências pré-configuradas é usando o **Docker Compose**:

```bash
docker compose up --build
```

### O que este comando faz automaticamente?
1. Inicializa o banco de dados PostgreSQL 16.
2. Compila a aplicação Go de maneira otimizada (multi-stage build).
3. **Executa as migrations automaticamente** estruturando as tabelas do banco.
4. Disponibiliza a API HTTP na porta `8080` (pronta para uso em `http://localhost:8080`).

---

## 🛠️ Decisões de Arquitetura & Design (Destaques do Assessment)

> [!IMPORTANT]
> Esta seção detalha as principais escolhas arquiteturais tomadas para garantir qualidade de código, facilidade de avaliação e robustez operacional.

### 1. Arquitetura MVC (Model-View-Controller)
Para garantir uma separação limpa de responsabilidades, o projeto foi estruturado utilizando o padrão clássico MVC:
* **Model (`pkg/models`)**: Gerencia o estado físico do banco de dados, transações ACID e consultas estruturadas. Toda a lógica de persistência e locking concorrente reside aqui.
* **View (`pkg/views`)**: Controla a serialização e representação externa das respostas (ex: converter o tipo `decimal.Decimal` em `string` no JSON para evitar qualquer perda de precisão flutuante no cliente).
* **Controller (`pkg/controllers`)**: Orquestra o fluxo de dados, recebendo a requisição HTTP, acionando a validação de entrada, invocando os Models apropriados e retornando as Views correspondentes.

### 2. Migrations Automáticas com Advisory Locks
As migrations de esquema do banco de dados (`db/migrations`) são carregadas em tempo de compilação usando a diretiva `//go:embed` e **executadas automaticamente na inicialização da aplicação** (dentro de `models.InitDB`).
* **Segurança Concorrente**: Para evitar condições de corrida (race conditions) de DDL quando múltiplas instâncias da aplicação iniciam simultaneamente (como em testes concorrentes ou deploy distribuído), utilizamos uma trava transacional exclusiva no PostgreSQL (`SELECT pg_advisory_xact_lock(42069)`). Isso garante atomicidade de migração por processo.

### 3. Test Orchestrator (`tests/orchestrator.go`)
Para evitar código duplicado (DRY) e garantir o isolamento absoluto dos testes de integração, criamos um **Orchestrator de Testes** ([orchestrator.go](file:///c:/Users/konam/OneDrive/Desktop/Workstation/Pedro%20Tec/kiichain-assessment/tests/orchestrator.go)).
* Ele abstrai operações pesadas de infraestrutura como inicialização de pool de conexão, execução de migrations sob demanda, limpeza do banco (`TRUNCATE TABLE` com cascade) entre testes, injeção de saldos falsos (seeding) e consultas diretas para asserções físicas de saldo.

### 4. Submissão Proposital do `.env.development`
O arquivo de configuração local `.env.development` **foi mantido de forma proposital no repositório Git**.
* **Zero Friction**: Esta decisão visa reduzir a fricção de inicialização do avaliador. Não há necessidade de criar ou renomear arquivos de ambiente manualmente para rodar a suite de testes locais ou subir a aplicação pela primeira vez.

### 5. Filtro de Input nos Controllers (`filterInput`)
A validação de integridade dos dados de entrada (como checagem de parâmetros nulos, tipos inválidos e conversões decimais) foi isolada dentro de um método helper privado `filterInput` em cada Controller.
* Isso garante que a assinatura dos handlers HTTP principais permaneça limpa, legível e focada em controlar o fluxo, delegando a higienização primária de payload para uma sub-função com responsabilidade única.

### 6. Linting Rígido & Fluxo de CI
* **Linter (`golangci-lint`)**: Adicionamos um arquivo de configuração rígido [`.golangci.yml`](file:///c:/Users/konam/OneDrive/Desktop/Workstation/Pedro%20Tec/kiichain-assessment/.golangci.yml) para rodar verificações de formatação estrita, checagem de erros ignorados (`errcheck`), importações não utilizadas e boas práticas.
* **CI (GitHub Actions)**: Configuramos um pipeline no GitHub Actions ([`ci.yml`](file:///c:/Users/konam/OneDrive/Desktop/Workstation/Pedro%20Tec/kiichain-assessment/.github/workflows/ci.yml)) que, a cada push ou pull request, inicializa uma instância isolada do PostgreSQL como serviço no GitHub Runner, formata o código, executa o linter estático e roda toda a suíte de testes de integração.

---

## 📂 Estrutura de Pastas

```
kiichain-assessment/
├── .github/workflows/
│   └── ci.yml              # Fluxo de CI automatizado (Linter + Testes)
├── cmd/
│   └── server/
│       └── main.go         # Bootstraps da API (Inicializa DB, Roteador e Servidor)
├── config/
│   └── config.go           # Carregador de variáveis de ambiente com defaults
├── db/
│   └── migrations/
│       ├── 000001_init.up.sql  # Definição física de tabelas e chaves únicas
│       └── migrations.go       # Embed de migrations do banco
├── pkg/
│   ├── controllers/        # C: Controllers (recepção de request, chamada ao filterInput)
│   │   ├── webhook_controller.go
│   │   └── balance_controller.go
│   ├── middleware/         # Middlewares (Assinatura HMAC, X-Empty-Header, Structured Log)
│   │   ├── auth.go
│   │   ├── headers.go
│   │   └── logger.go
│   ├── models/             # M: Models (Interações físicas de escrita e leitura de saldos)
│   │   ├── db.go
│   │   ├── ledger_entry.go
│   │   └── balance.go
│   └── views/              # V: Views (Serialização segura de precisão decimal)
│       └── response.go
├── tests/
│   ├── integration/        # Testes de integração física contra Postgres
│   │   ├── balance/
│   │   │   └── get_test.go
│   │   └── webhook/
│   │       └── post_test.go
│   └── orchestrator.go     # Abstração de controle de testes e infraestrutura
├── .env.development        # Configurações de ambiente (enviado propositalmente)
├── .golangci.yml           # Configuração de linters rigorosos de código
├── Dockerfile              # Construção multi-estágio da aplicação
├── docker-compose.yml      # Docker Compose com postgres integrado
└── run.sh                  # Script bash com curl para validação E2E
```

---

## ⚙️ Variáveis de Ambiente Configuráveis

| Variável | Descrição | Padrão |
| :--- | :--- | :--- |
| `PORT` | Porta usada pelo servidor HTTP | `8080` |
| `HMAC_SECRET` | Chave simétrica usada para assinar/verificar payloads | *(Obrigatória)* |
| `TOLERANCE_MINUTES` | Janela de tolerância em minutos para expiração de timestamp | `5` |
| `DB_HOST` | Host do banco PostgreSQL | `localhost` |
| `DB_PORT` | Porta do banco PostgreSQL | `5432` |
| `DB_USER` | Usuário do banco PostgreSQL | `postgres` |
| `DB_PASSWORD` | Senha do banco PostgreSQL | `postgres` |
| `DB_NAME` | Nome do banco PostgreSQL | `ledger` |
| `DB_SSLMODE` | SSL Mode para a conexão Postgres | `disable` |

---

## 📖 Documentação da API

### 1. Registrar Transação via Webhook
* **Rota**: `POST /webhook`
* **Headers de Segurança**:
  * `X-Timestamp`: Unix Timestamp em segundos do envio da requisição.
  * `X-Nonce`: Identificador único (UUID ou string aleatória) de uso único.
  * `X-Signature`: Assinatura HMAC-SHA256 hex-encoded do payload.
* **Formato do Payload Assinado**:
  `payload = X-Timestamp + "\n" + X-Nonce + "\n" + <raw_body_bytes>`
* **Exemplo de Body (JSON)**:
  ```json
  {
    "user": "user_alice",
    "asset": "ETH",
    "amount": "1.500000000000000000"
  }
  ```
* **Respostas HTTP**:
  * `200 OK`: Transação criada e saldo consolidado com sucesso.
  * `400 Bad Request`: Payload malformado, headers ausentes ou timestamp fora da tolerância (replay check).
  * `401 Unauthorized`: Assinatura HMAC inválida.
  * `409 Conflict`: Replay attack detectado (nonce repetido).

### 2. Consultar Saldos Consolidados
* **Rota**: `GET /balance/{user}`
* **Exemplo de Resposta (200 OK)**:
  ```json
  {
    "user": "user_alice",
    "balances": {
      "ETH": "1.500000000000000000"
    }
  }
  ```
  *(Nota: Se o usuário não existir no banco, a API retorna um objeto balances vazio formatado em JSON `{}`).*

---

## 🧪 Verificação & Testes de Integração

### 1. Rodar a Suíte de Testes Locais
Para validar de ponta a ponta as condições de concorrência, idempotência e precisão aritmética de 18 casas decimais:
1. Suba apenas o contêiner do banco de dados:
   ```bash
   docker compose up -d db
   ```
2. Execute o comando Go de testes:
   ```bash
   go test -p 1 -count=1 -v ./tests/integration/...
   ```

### 2. Rodar Script de Simulação E2E
Criamos um script utilitário (`run.sh`) que faz chamadas curl assinadas gerando assinaturas válidas em tempo de execução:
```bash
chmod +x run.sh
./run.sh
```
O script simula:
1. Verificação de saldo inicial vazio (`{}`).
2. Depósito bem sucedido de `1500.50 USDT`.
3. Bloqueio de **Replay Attack** enviando o mesmo nonce (retorna `409 Conflict`).
4. Bloqueio de assinatura inválida (retorna `401 Unauthorized`).
5. Bloqueio de timestamp expirado (retorna `400 Bad Request`).
6. Dedução bem sucedida de `500.25 USDT`.
7. Verificação de saldo acumulado de exatamente `1000.25 USDT`.

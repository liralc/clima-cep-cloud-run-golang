# Clima por CEP — Google Cloud Run

API em Go que recebe um CEP brasileiro, identifica a cidade via [ViaCEP](https://viacep.com.br) e retorna a temperatura atual (via [WeatherAPI](https://www.weatherapi.com)) em Celsius, Fahrenheit e Kelvin.

## URL no Cloud Run

> **https://clima-cep-55769039268.southamerica-east1.run.app**

Exemplos:

```bash
curl -i https://clima-cep-55769039268.southamerica-east1.run.app/weather/01001000   # 200
curl -i https://clima-cep-55769039268.southamerica-east1.run.app/weather/123        # 422
curl -i https://clima-cep-55769039268.southamerica-east1.run.app/weather/99999999   # 404
```

## API

`GET /weather/{cep}` — `{cep}` deve conter exatamente 8 dígitos (sem hífen).

| Cenário | Status | Corpo |
|---|---|---|
| Sucesso | `200` | `{"temp_C":28.5,"temp_F":83.3,"temp_K":301.5}` |
| CEP com formato inválido | `422` | `invalid zipcode` |
| CEP inexistente | `404` | `can not find zipcode` |
| Falha em serviço externo | `500` | `internal server error` |

Conversões: `F = C × 1.8 + 32` e `K = C + 273` (valores arredondados para 2 casas decimais).

## Configuração

| Variável | Obrigatória | Descrição |
|---|---|---|
| `WEATHER_API_KEY` | sim | Chave da WeatherAPI (gratuita em https://www.weatherapi.com/signup.aspx) |
| `PORT` | não | Porta HTTP (padrão `8080`; definida automaticamente pelo Cloud Run) |

## Rodando localmente com Docker

```bash
cp .env.example .env          # edite e informe sua WEATHER_API_KEY
docker compose up --build
```

Ou sem compose:

```bash
docker build -t clima-cep .
docker run --rm -p 8080:8080 -e WEATHER_API_KEY=<sua-chave> clima-cep
```

Testando:

```bash
curl -i http://localhost:8080/weather/01001000   # 200
curl -i http://localhost:8080/weather/123        # 422
curl -i http://localhost:8080/weather/99999999   # 404
```

## Rodando os testes

Via Docker (não requer Go instalado):

```bash
docker build --target test .
```

Com Go instalado (1.27+):

```bash
go test ./...
go test -race ./...   # requer CGO
```

Os testes não acessam serviços reais: ViaCEP e WeatherAPI são simulados com `httptest`.

## Deploy no Google Cloud Run

A chave da WeatherAPI fica no Secret Manager e é exposta ao container como a variável `WEATHER_API_KEY`.

```bash
# 1. Criar o secret (lê a chave do .env, sem expô-la no histórico do shell)
gcloud services enable secretmanager.googleapis.com
set -a && . ./.env && set +a
printf '%s' "$WEATHER_API_KEY" | gcloud secrets create weather-api-key --data-file=-

# 2. Permitir que a conta de serviço do Cloud Run leia o secret
gcloud secrets add-iam-policy-binding weather-api-key \
  --member="serviceAccount:<PROJECT_NUMBER>-compute@developer.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

# 3. Deploy
gcloud run deploy clima-cep \
  --source . \
  --region southamerica-east1 \
  --allow-unauthenticated \
  --set-secrets WEATHER_API_KEY=weather-api-key:latest
```

Ao final, o comando exibe a URL do serviço.

Para trocar a chave, adicione uma nova versão do secret e crie uma nova revisão (o valor `latest` é resolvido quando a instância inicia):

```bash
printf '%s' "<nova-chave>" | gcloud secrets versions add weather-api-key --data-file=-
gcloud run services update clima-cep --region southamerica-east1 \
  --set-secrets WEATHER_API_KEY=weather-api-key:latest
```

## Estrutura

```
cmd/server/            ponto de entrada, configuração e graceful shutdown
internal/handler/      handler HTTP e rotas
internal/location/     validação de CEP e cliente ViaCEP
internal/weather/      cliente WeatherAPI
internal/temperature/  conversões de temperatura
```

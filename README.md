# precos-core

Contrato de ingestão, schema do banco e espinha comum dos workers da plataforma de comparação de
preços de supermercados.

Este repositório é o **dono exclusivo do schema**: os workers de coleta entregam `[]domain.Item` e
não conhecem nomes de tabela nem escrevem SQL.

## Uso

```bash
docker compose up -d --wait
export DATABASE_URL=postgres://precos:precos@localhost:5432/precos?sslmode=disable
go run ./cmd/precosctl migrate
go run ./cmd/precosctl stats
go run ./cmd/precosctl prune
```

**O banco local sobe por aqui, e não por cada worker.** O compose prefixa volumes com o nome do
projeto: um compose por worker criaria um banco por worker, e a comparação nunca veria dois mercados
juntos. O volume tem nome fixo, `precos-pgdata`.

**Só o `precosctl` aplica migrations.** Worker nenhum migra: isso causaria corrida e versões
divergentes de schema entre os repositórios.

| Comando | O que faz |
|---|---|
| `migrate [--down]` | aplica (ou reverte) as migrations |
| `stats` | contagens, órfãos, preços por loja, produtos em mais de uma loja, últimas execuções |
| `prune` | apaga produtos que nenhum preço referencia — sobram quando uma regra de identidade muda |

## Modelo

Duas tabelas de dado, sem camada intermediária de "anúncio na loja":

- **`products`** — o produto: EAN, nome e imagem canônicos.
- **`prices`** — uma linha por produto **por mercado**, sobrescrita a cada coleta. Os campos que
  variam por mercado (código interno do SKU, nome como aquele mercado escreve, link da página)
  moram aqui.

`products.match_key` é a chave determinística de upsert: o EAN quando houver, senão
`<store_id>:<store_sku_key>`.

### O que conta como EAN

Workers entregam o código de barras **cru**; o core decide com `domain.NormalizeEAN`. Só vale GTIN
de 8, 12, 13 ou 14 dígitos com dígito verificador correto. GTIN-12 (UPC-A) vira GTIN-13 com zero à
esquerda, e GTIN-14 que começa com zero perde o zero, para que o mesmo item case entre mercados.

Códigos de **11 dígitos** ganham o zero à esquerda antes da validação: são UPC-A que perderam o zero
em algum sistema que guardou o código como número. Em 13/09/2026, 223 de 223 deles passavam no
verificador assim — ao acaso seriam ~10%.

Mercados mandam coisas que não são EAN nesse campo — o PLU dos pesáveis (`1113`, `26185`). Aceitos,
eles virariam chave compartilhada e fundiriam produtos
diferentes de mercados diferentes na mesma linha.

## A consulta que é o objetivo do projeto

```sql
select s.name, p.name_at_store, p.price_cents, p.club_price_cents, p.product_url
from products pr
join prices p on p.product_id = pr.id
join stores s on s.id = p.store_id
where pr.ean = $1 and p.is_available
order by p.price_cents;
```

## Ingestão

`Ingester.Submit` grava a coleta inteira numa transação. Como os dois passos são upsert,
`CopyFrom` não serve direto (não faz `ON CONFLICT`): o caminho é `CopyFrom` para tabela temporária
e um `INSERT ... SELECT ... ON CONFLICT DO UPDATE` a partir dela — uma ida ao banco em vez de
quinze mil.

## `collect`: a espinha dos workers

Um worker implementa `collect.Source` e chama `collect.Main`:

```go
type Source interface {
    Sections(ctx context.Context) ([]Section, error)
    IterPages(ctx context.Context, s Section, fn func(Page) error) error
    Requests() int
}
```

A fonte descobre as seções, pagina cada uma e entrega páginas com o JSON bruto e os itens já
mapeados. O resto mora aqui, uma vez só:

| Arquivo | Papel |
|---|---|
| `pipeline.go` | seções → páginas → dump → dedupe → sanidade → gravação; a run sempre fecha, inclusive no Ctrl+C |
| `sanity.go` | reprova coleta vazia ou com queda acima de 20% frente à última completa |
| `rawstore.go` | dump `.json.gz` com chave determinística |
| `httpx.go` | timeout, `User-Agent`, intervalo entre requisições, retry só em 5xx; 4xx vira `*StatusError` |
| `money.go` | `ToCents` a partir da string, nunca por `float64` |
| `config.go` | `.env`, `DATABASE_URL`, `STORE_ID`, `REQUEST_DELAY`, `RAW_DIR` |
| `cli.go` | `run [--dry-run] [--category a,b]`, sinais, logs em JSON, resumo, código de saída |

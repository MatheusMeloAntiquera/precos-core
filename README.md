# precos-core

Contrato de ingestão e schema do banco compartilhado da plataforma de comparação de preços de
supermercados.

Este repositório é o **dono exclusivo do schema**: os workers de coleta entregam `[]domain.Item` e
não conhecem nomes de tabela nem escrevem SQL.

## Uso

```bash
export DATABASE_URL=postgres://precos:precos@localhost:5432/precos?sslmode=disable
go run ./cmd/precosctl migrate
go run ./cmd/precosctl stats
```

**Só este binário aplica migrations.** Worker nenhum migra: isso causaria corrida e versões
divergentes de schema entre os repositórios.

## Modelo

Duas tabelas de dado, sem camada intermediária de "anúncio na loja":

- **`products`** — o produto: EAN, nome e imagem canônicos.
- **`prices`** — uma linha por produto **por mercado**, sobrescrita a cada coleta. Os campos que
  variam por mercado (código interno do SKU, nome como aquele mercado escreve, link da página)
  moram aqui.

`products.match_key` é a chave determinística de upsert. O EAN não serve sozinho porque itens de
peso (hortifrúti, açougue, padaria) vêm sem código de barras; nesses casos a chave cai para
`<store_id>:<store_sku_key>`.

Com um mercado só, cada item vira uma linha. Quando o segundo entrar, os itens **com EAN** casam
sozinhos; os sem EAN ficam separados — que é o problema de casamento entre catálogos adiado de
propósito até haver dois catálogos reais na mão.

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

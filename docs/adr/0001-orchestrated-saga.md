# ADR 0001: Saga Orquestrada (nao Coreografada)

## Status
Aceito

## Contexto
Este servico e um dos quatro microsservicos extraidos de um sistema de PDV
monolitico para uma oficina mecanica. Criar e cancelar uma Ordem de Servico
(OS) e apenas uma etapa de uma transacao de negocio maior, que atravessa
varios servicos (orcamento, pagamento, execucao), e que precisa poder ser
desfeita de forma consistente caso uma etapa posterior falhe. E preciso
decidir se essa transacao entre servicos e coordenada por um orquestrador
central (`pos-saga-orchestrator`), que emite comandos e reage a eventos, ou
se e puramente coreografada, com os servicos reagindo diretamente aos
eventos de dominio uns dos outros, sem coordenador central.

## Decisao
Este servico participa de uma **saga orquestrada**. Ele emite eventos de
dominio (`OSCreated`, `OSCancelled`) que o orquestrador consome, e consome
comandos de compensacao vindos do orquestrador (`CancelOSCommand`), em vez
de reagir diretamente a eventos de dominio emitidos por servicos irmaos. O
orquestrador e o dono da definicao da saga e mantem um log de auditoria
(`saga_history`) de cada etapa e compensacao entre os quatro servicos.

## Racional
- **Auditabilidade**: um time pequeno construindo isso para um desafio
  avaliado precisa de um lugar unico para responder "o que aconteceu com
  essa ordem, e por que", sem precisar reconstruir a causalidade a partir de
  quatro fluxos de eventos independentes.
- **Logica de compensacao mais simples**: este servico so precisa saber
  reagir a um comando (`CancelOSCommand`); nao precisa entender os eventos
  de dominio dos outros tres servicos para decidir quando compensar.
- **Caminho de rollback demonstravel**: o orquestrador consegue conduzir e
  demonstrar visivelmente, de forma sincrona, um rollback completo
  (por exemplo: falha de pagamento -> cancelamento da OS) de ponta a ponta,
  o que e importante para um cenario de demo/avaliacao ao vivo.
- **Trade-off aceito**: o orquestrador se torna um ponto unico de
  coordenacao e um possivel gargalo/ponto unico de falha. Dado o tamanho
  pequeno do time, o escopo limitado (4 servicos) e o valor de uma trilha de
  auditoria clara para a avaliacao, esse e um trade-off aceitavel frente a
  complexidade operacional de uma coreografia pura (acoplamento implicito
  via contratos de evento, cadeias de compensacao mais dificeis de
  rastrear).

## Consequencias
- Este servico precisa implementar o padrao outbox para publicar de forma
  confiavel os eventos `OSCreated`/`OSCancelled` em resposta aos comandos
  esperados pelo orquestrador.
- Este servico precisa implementar tratamento idempotente de comandos
  (`ProcessedEventRepository`), ja que a entrega "at-least-once" do
  orquestrador pode reenviar o `CancelOSCommand`.
- O acoplamento com o vocabulario de comandos do orquestrador e explicito e
  versionado atraves do envelope de evento compartilhado (`event_name`,
  `event_version`).

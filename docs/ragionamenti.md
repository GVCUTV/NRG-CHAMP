// v2
// docs/ragionamenti.md
## Kafka
### Device -> aggregator
Ogni zona deve avere il suo proprio topic dedicato, denominato con lo stesso id della zona, ogni device deve avere la sua propria partizione denominata anche essa con l’id del device per evitare collisioni dovute all’alta frequenza di scrittura dei device.
L'istanza di aggregator assegnata a quella zona legge in round robin dalle partizioni, etichettando come letti i messaggi, e passa alla prossima partizione appena trova un messaggio dell'epoca successiva, che non etichetta come letto.
Ogni istanza di aggregator esegue anche un round robin di livello superiore sui topic a lui assegnati.

L'aggregator:
- rimuove ogni overhead di comunicazione
- scarta gli outlier
- aggrega i dati provenienti dai dispositivi della stessa zona


### Aggregator -> MAPE
Ogni aggregator scrive su un topic, utilizzando una partizione per ogni zona

Ogni aggregator è assegnato a un'istanza di MAPE (1:1), la quale fa round robin sulle partizioni e, per ogni partizione, leggi tutti i messaggi presenti e lavora solo il più recente, scartando gli altri in quanto obsoleti.

### MAPE -> attuatori
Ogni MAPE, gestendo più zone, scrive su un topic dedicato alla zona, mentre le partizioni fanno riferimento agli attuatori di quella zona. Ogni attuatore legge dalla propria partizione tutti i messaggi presenti, ma lavora solo il più recente, scartando gli altri.

### Aggregator + MAPE -> ledger
C'è un topic per ogni zona. Ogni topic ha due partizioni: aggregator e mape. Il ledger si occupa di fare il matching tra i dati sulle due partizioni, in base all'epoca. Se manca qualche elemento, lo stima (vedere come) dalle epoche precedenti e successiva.

## Implementazione Circuit Breaker
Ci sarà un modulo dedidato e condiviso tra tutti i servizi atto all'implementazione del circuit breaker.
In particolare, il modulo conterrà un gestore per ogni tipo di chiamata (HTTP, Kafka, ecc.).
Ogni volta che verrà inviata una richiesta a un servizio remoto, l'invocazione dovrà passare per il gestore
relativo alla chiamata all'interno del modulo.

I gestori delle chiamate eseguiranno le seguenti operazioni:
- invia la richiesta
- se la richiesta non va a buon fine ed è stato raggiunto il limite massimo di fallimenti, attende X secondi prima di ricontattare il servizio remoto
- passati gli X secondi, invia una richiesta di test al servizio; se va a buon fine, ripete l'invio della richiesta originale
- resetta il contatore dei fallimenti

Il numero massimo di fallimenti e il tempo di reset del circuit braker dovranno assere impostabili tramite un file di properties

## MAPE
Gli obiettivi di temperatura all'interno di ogni zona devono essere scritti all'interno di un file di properties.
Ogni zona può definire un override dedicato (`target.<zoneId>=<float>`) che viene caricato all'avvio; a runtime
le API HTTP `/config/temperature` permettono di riallineare i setpoint in memoria senza modificare il file.
Le modifiche vengono mantenute solo finché il servizio resta attivo, così da evitare conflitti di editing; isteresi e curve
ventola restano globali finché non emergono requisiti di personalizzazione specifici.
### Monitor

### Analyze

### Plan
Se la temperatura è diversa da quella impostata, segnala di accendere o spegnere il riscaldamento o il raffreddamento, anche in base all'isteresi impostata del file di properties. In base alla differenza di temperatura, imposta la velocità delle ventole.
### Execute

## Ledger
Il ledger si occupa di fare il matching tra i dati sulle due partizioni di ogni topic (corrispondenti alle zone), in base all'epoca. Se manca qualche elemento, lo stima (vedere come) dalle epoche precedenti e successiva. Successivamente, inserisce i dati matchati nel blocco attualmente in lavorazione della blockchain.

Ogni qualvolta l'area dati di un blocco raggiunge il limite, si calcola l'hash dei dati del blocco, lo aggiunge al suo header, ne calcola l'hash totale, apre un nuovo blocco e inserisce nell'header l'hash dell'header del blocco precedente. Si continua poi a scrivere su tale blocco.

Il ledger contiene anche una mappatura dei blocchi che contengono le informazioni delle singole zone. (da ragionarci).

### Razionale per il feed pubblico
La replica pubblica degli epoch utilizza lo schema minimale `epoch.public` (vedere documentazione tecnica) perché permette di:

* **Limitare i dati sensibili:** il campo `aggregator.summary` contiene solo roll-up stabili (medie, totali normalizzati, rapporti), evitando di esporre misure grezze per device o informazioni identificative.
* **Stabilizzare il contratto:** mantenendo un dizionario di metriche aggregate, l'evoluzione diventa puramente additiva. I consumatori (Gamification in primis) possono negoziare nuove chiavi senza dover riconciliare payload profondi.
* **Collegare al ledger v2:** i metadati `block.height`, `headerHash` e `dataHash` consentono di risalire al blocco completo per audit o ricalcoli senza divulgare dati privati nello stream pubblico.

### Semantica di consegna e idempotenza
La pubblicazione Kafka del ledger è configurata in modalità *at-least-once*: il publisher attende gli ACK richiesti, ma in caso di retry una stessa epoca può essere spedita due volte. I consumatori devono quindi:

* Conservare un archivio di chiavi idempotenti `zoneId:epochIndex` (ad esempio in memoria con TTL o su archivio persistente) e scartare le ripetizioni prima di aggiornare punteggi o dashboard.
* Confermare l'offset del messaggio solo dopo che l'elaborazione è stata completata e il checkpoint/idempotency store è stato aggiornato, così da evitare perdite in caso di riavvio.
* Valutare la partizione partendo dalla chiave `zoneId` (default del publisher) per preservare l'ordine relativo di ogni zona quando si distribuisce il carico su più worker.

## Assessment
Segue un approccio pull dal ledger ogni qualvolta si vuole fare una verifica.

## Gamification
Segue un approccio push, ogni volta che il ledger scrive sulla blockchain, manda i dati anche a gamification. Calcola poi la media dei consumi tra tutti quelli che sono in gara; chi contribuisce ad abbassarla prende punti, chi ad alzarla ne perde.

------------------------------ MODULE Ledger ------------------------------
(***************************************************************************)
(* Protocolo de saga e idempotencia para transferencias entre dos cuentas. *)
(* Modelo abstracto del CONTRACT: el dinero en transito vive en `held`.    *)
(* Los estados, mensajes, reenvios e idempotencia coinciden con el Go.     *)
(***************************************************************************)
EXTENDS Integers, FiniteSets, TLC

CONSTANTS A, B, Amounts, InitBal

ASSUME A # B
ASSUME Amounts \subseteq Nat \ {0}
ASSUME InitBal \in Nat

Accounts == {A, B}
Keys     == {1, 2}
Kinds    == {"capture", "ack", "release"}

CaptureId(k) == 10 * k + 1
AckId(k)     == 10 * k + 2
ReleaseId(k) == 10 * k + 3

VARIABLES avail, held, saga, idemp, msgs, inbox

TypeOK ==
  /\ avail \in [Accounts -> Nat]
  /\ held  \in [Keys -> Nat]
  /\ saga  \in [Keys -> [s: {"none", "reserved", "settled", "released"},
                         from: Accounts, to: Accounts, amt: Amounts \cup {0}]]
  /\ idemp \in [Keys -> [used: BOOLEAN, from: Accounts, to: Accounts, amt: Amounts \cup {0}]]
  /\ msgs  \subseteq [id: Nat, kind: Kinds, key: Keys]
  /\ inbox \subseteq Nat

Init ==
  /\ avail = [x \in Accounts |-> InitBal]
  /\ held  = [k \in Keys |-> 0]
  /\ saga  = [k \in Keys |-> [s |-> "none", from |-> A, to |-> B, amt |-> 0]]
  /\ idemp = [k \in Keys |-> [used |-> FALSE, from |-> A, to |-> B, amt |-> 0]]
  /\ msgs  = {}
  /\ inbox = {}

Transfer(k, from, to, amt) ==
  /\ from # to
  /\ amt \in Amounts
  /\ IF idemp[k].used
     THEN /\ idemp[k].from = from
          /\ idemp[k].to   = to
          /\ idemp[k].amt  = amt
          /\ UNCHANGED <<avail, held, saga, idemp, msgs, inbox>>
     ELSE /\ avail[from] >= amt
          /\ avail' = [avail EXCEPT ![from] = @ - amt]
          /\ held'  = [held  EXCEPT ![k]    = amt]
          /\ saga'  = [saga  EXCEPT ![k]    =
                          [s |-> "reserved", from |-> from, to |-> to, amt |-> amt]]
          /\ idemp' = [idemp EXCEPT ![k]    =
                          [used |-> TRUE, from |-> from, to |-> to, amt |-> amt]]
          /\ msgs'  = msgs \union {[id |-> CaptureId(k), kind |-> "capture", key |-> k]}
          /\ UNCHANGED inbox

Conflict(k, from, to, amt) ==
  /\ idemp[k].used
  /\ \/ idemp[k].from # from
     \/ idemp[k].to   # to
     \/ idemp[k].amt  # amt
  /\ UNCHANGED <<avail, held, saga, idemp, msgs, inbox>>

Redeliver ==
  \E m \in msgs :
    /\ m.id \in inbox
    /\ UNCHANGED <<avail, held, saga, idemp, msgs, inbox>>

DeliverCapture ==
  \E m \in msgs :
    /\ m.kind = "capture"
    /\ m.id \notin inbox
    /\ saga[m.key].s = "reserved"
    /\ inbox' = inbox \union {m.id}
    /\ held'  = [held  EXCEPT ![m.key] = 0]
    /\ avail' = [avail EXCEPT ![saga[m.key].to] = @ + saga[m.key].amt]
    /\ msgs'  = msgs \union {[id |-> AckId(m.key), kind |-> "ack", key |-> m.key]}
    /\ UNCHANGED <<saga, idemp>>

DeliverAck ==
  \E m \in msgs :
    /\ m.kind = "ack"
    /\ m.id \notin inbox
    /\ saga[m.key].s = "reserved"
    /\ inbox' = inbox \union {m.id}
    /\ saga'  = [saga EXCEPT ![m.key].s = "settled"]
    /\ UNCHANGED <<avail, held, idemp, msgs>>

DeliverRelease ==
  \E m \in msgs :
    /\ m.kind = "release"
    /\ m.id \notin inbox
    /\ saga[m.key].s = "reserved"
    /\ inbox' = inbox \union {m.id}
    /\ held'  = [held  EXCEPT ![m.key] = 0]
    /\ avail' = [avail EXCEPT ![saga[m.key].from] = @ + saga[m.key].amt]
    /\ saga'  = [saga EXCEPT ![m.key].s = "released"]
    /\ UNCHANGED <<idemp, msgs>>

\* Camino antagonista: el capture se descarta (cuenta congelada / inexistente)
\* y se emite el release, como en el codigo Go.
ChooseRelease ==
  \E m \in msgs :
    /\ m.kind = "capture"
    /\ m.id \notin inbox
    /\ saga[m.key].s = "reserved"
    /\ inbox' = inbox \union {m.id}
    /\ msgs'  = msgs \union {[id |-> ReleaseId(m.key), kind |-> "release", key |-> m.key]}
    /\ UNCHANGED <<avail, held, saga, idemp>>

Next ==
  \/ \E k \in Keys, from \in Accounts, to \in Accounts, amt \in Amounts :
        Transfer(k, from, to, amt) \/ Conflict(k, from, to, amt)
  \/ Redeliver \/ DeliverCapture \/ DeliverAck \/ DeliverRelease \/ ChooseRelease

vars == <<avail, held, saga, idemp, msgs, inbox>>

Spec == Init /\ [][Next]_vars

MoneyOK == avail[A] + avail[B] + held[1] + held[2] = 2 * InitBal

NoOverdraft == \A x \in Accounts : avail[x] >= 0

TerminalWhenQuiet ==
  (\A m \in msgs : m.id \in inbox) =>
    \A k \in Keys : saga[k].s \in {"none", "settled", "released"}

IdempOnce ==
  \A k \in Keys :
    idemp[k].used => saga[k].s \in {"reserved", "settled", "released"}

Inv == TypeOK /\ MoneyOK /\ NoOverdraft /\ TerminalWhenQuiet /\ IdempOnce
=============================================================================

# Agent Payments Protocol (AP2) Integration with AEX

## Executive Summary

This document describes the integration of the **Agent Payments Protocol (AP2)** with the **Agent Exchange (AEX)** platform. AP2 is an open-source protocol by Google that enables AI agents to securely execute autonomous financial transactions. By integrating AP2, AEX can facilitate payment flows between consumer agents and provider agents through the existing marketplace infrastructure.

---

## Part 1: Understanding AP2

### 1.1 What is AP2?

AP2 (Agent Payments Protocol) is an open standard designed to solve a critical problem: **existing payment infrastructure was built for human-initiated transactions, not autonomous AI agents**. AP2 provides:

- **Cryptographic verification** of agent authority to transact
- **Non-repudiable proof** of user intent through signed mandates
- **Clear accountability** for dispute resolution
- **Interoperability** across different agent frameworks (A2A, MCP)

### 1.2 Key Actors in AP2

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           AP2 ECOSYSTEM ACTORS                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────┐    ┌──────────────────┐    ┌─────────────────────────┐        │
│  │  User   │───▶│ Shopping Agent   │───▶│   Merchant Endpoint     │        │
│  │ (Human) │    │ (SA)             │    │   (ME)                  │        │
│  └─────────┘    └────────┬─────────┘    └───────────┬─────────────┘        │
│       │                  │                          │                       │
│       │                  ▼                          ▼                       │
│       │         ┌──────────────────┐    ┌─────────────────────────┐        │
│       └────────▶│ Credentials      │    │ Merchant Payment        │        │
│                 │ Provider (CP)    │◀───│ Processor (MPP)         │        │
│                 └──────────────────┘    └─────────────────────────┘        │
│                          │                          │                       │
│                          ▼                          ▼                       │
│                 ┌──────────────────────────────────────────────┐           │
│                 │        Payment Network & Issuer              │           │
│                 └──────────────────────────────────────────────┘           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

| Actor | Role | Description |
|-------|------|-------------|
| **User** | Human Authority | Initiates tasks and provides financial authorization |
| **Shopping Agent (SA)** | User's AI Agent | Understands user needs, discovers products, negotiates with merchants |
| **Credentials Provider (CP)** | Wallet/Payment Manager | Securely manages payment credentials and executes payments |
| **Merchant Endpoint (ME)** | Seller Interface | Offers products/services, creates carts, fulfills orders |
| **Merchant Payment Processor (MPP)** | Payment Handler | Constructs transaction authorization messages |
| **Network & Issuer** | Payment Infrastructure | Payment networks and credential issuers |

### 1.3 The Three Mandates (Verifiable Digital Credentials)

AP2's core innovation is the use of cryptographically signed **mandates** that create non-repudiable proof of intent:

#### 1.3.1 Intent Mandate
```json
{
  "natural_language_description": "Buy concert tickets under $500, close to stage",
  "merchants": ["ticketmaster.com"],
  "requires_refundability": true,
  "intent_expiry": "2026-02-15T00:00:00Z",
  "user_cart_confirmation_required": false
}
```
- **Purpose**: Captures user's shopping intent in natural language
- **When**: Created when user delegates a task to the agent
- **Signed by**: User (hardware-backed device key)

#### 1.3.2 Cart Mandate
```json
{
  "contents": {
    "id": "cart_12345",
    "payment_request": {
      "method_data": [{"supported_methods": "CARD"}],
      "details": {
        "display_items": [
          {"label": "Concert Ticket - Row A", "amount": {"currency": "USD", "value": 450.00}}
        ],
        "total": {"label": "Total", "amount": {"currency": "USD", "value": 450.00}}
      }
    }
  },
  "merchant_authorization": "eyJhbGciOiJSUzI1NiI..."
}
```
- **Purpose**: Binds specific SKUs, prices, and fulfillment terms
- **When**: Created when merchant confirms they can fulfill the order
- **Signed by**: Merchant first, then optionally by user

#### 1.3.3 Payment Mandate
```json
{
  "payment_mandate_contents": {
    "payment_mandate_id": "pm_67890",
    "payment_details_total": {"currency": "USD", "value": 450.00},
    "payment_response": {
      "method_name": "CARD",
      "details": {"token": "tok_xyz789"}
    },
    "merchant_agent": "TicketMerchant"
  },
  "user_authorization": "eyJhbGciOiJFUzI1NksI..."
}
```
- **Purpose**: Provides payment ecosystem visibility into agentic transactions
- **When**: Created at payment execution time
- **Contains**: AI agent presence signals, payment method tokens

### 1.4 Transaction Flow (Human-Present)

```
┌───────────────────────────────────────────────────────────────────────────────────┐
│                        AP2 HUMAN-PRESENT TRANSACTION FLOW                         │
└───────────────────────────────────────────────────────────────────────────────────┘

User              Shopping Agent        Credentials Provider      Merchant Agent
 │                     │                        │                       │
 │  1. Shopping task   │                        │                       │
 │────────────────────▶│                        │                       │
 │                     │                        │                       │
 │  2. Confirm intent  │                        │                       │
 │◀────────────────────│                        │                       │
 │                     │                        │                       │
 │  3. "Yes, proceed"  │                        │                       │
 │────────────────────▶│                        │                       │
 │                     │                        │                       │
 │                     │  4. Get payment methods│                       │
 │                     │───────────────────────▶│                       │
 │                     │◀───────────────────────│                       │
 │                     │  5. {payment methods}  │                       │
 │                     │                        │                       │
 │                     │  6. IntentMandate      │                       │
 │                     │────────────────────────────────────────────────▶│
 │                     │                        │                       │
 │                     │                        │    7. Create cart &   │
 │                     │                        │       sign CartMandate│
 │                     │                        │                       │
 │                     │  8. {signed CartMandate}                       │
 │                     │◀────────────────────────────────────────────────│
 │                     │                        │                       │
 │  9. Show cart       │                        │                       │
 │◀────────────────────│                        │                       │
 │                     │                        │                       │
 │ 10. Select payment  │                        │                       │
 │────────────────────▶│                        │                       │
 │                     │                        │                       │
 │                     │ 11. Get payment token  │                       │
 │                     │───────────────────────▶│                       │
 │                     │◀───────────────────────│                       │
 │                     │ 12. {token}            │                       │
 │                     │                        │                       │
 │ 13. Confirm purchase│                        │                       │
 │    (device attestation)                      │                       │
 │────────────────────▶│                        │                       │
 │                     │                        │                       │
 │                     │ 14. PaymentMandate + purchase                  │
 │                     │────────────────────────────────────────────────▶│
 │                     │                        │                       │
 │                     │                        │  15. Process payment  │
 │                     │                        │◀──────────────────────│
 │                     │                        │──────────────────────▶│
 │                     │                        │                       │
 │                     │ 16. Payment receipt    │                       │
 │                     │◀────────────────────────────────────────────────│
 │ 17. Purchase complete                        │                       │
 │◀────────────────────│                        │                       │
 │                     │                        │                       │
```

### 1.5 AP2 A2A Extension

AP2 integrates with A2A protocol through an extension. Agents declare their AP2 roles in their Agent Card:

```json
{
  "name": "MerchantAgent",
  "capabilities": {
    "extensions": [
      {
        "uri": "https://github.com/google-agentic-commerce/ap2/tree/v0.1",
        "params": {
          "roles": ["merchant"]
        }
      }
    ]
  },
  "skills": [
    {"id": "search_catalog", "name": "Search Catalog"},
    {"id": "create_cart", "name": "Create Cart"}
  ]
}
```

**AP2 Roles:**
- `merchant` - Offers products/services, creates CartMandates
- `shopper` - Acts on user's behalf to find and purchase items
- `credentials-provider` - Manages user's payment credentials
- `payment-processor` - Processes payment transactions

---

## Part 2: AEX-AP2 Integration Design

### 2.1 Current AEX Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           CURRENT AEX ARCHITECTURE                              │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  Consumer Agent                    AEX Platform                  Provider Agent │
│  ┌──────────────┐    ┌───────────────────────────────────┐    ┌──────────────┐ │
│  │              │    │  ┌─────────────────────────────┐  │    │              │ │
│  │  Legal Agent │───▶│  │      Gateway API            │  │◀───│  Legal Agent │ │
│  │  (Consumer)  │    │  └─────────────────────────────┘  │    │  (Provider)  │ │
│  │              │    │              │                    │    │              │ │
│  └──────────────┘    │  ┌───────────┴───────────┐        │    └──────────────┘ │
│                      │  │                       │        │                     │
│                      │  ▼                       ▼        │                     │
│                      │  ┌────────┐    ┌─────────────┐    │                     │
│                      │  │ Work   │    │    Bid      │    │                     │
│                      │  │Publisher│   │   Gateway   │    │                     │
│                      │  └────────┘    └─────────────┘    │                     │
│                      │       │               │           │                     │
│                      │       ▼               ▼           │                     │
│                      │  ┌──────────────────────────┐     │                     │
│                      │  │     Bid Evaluator        │     │                     │
│                      │  └──────────────────────────┘     │                     │
│                      │              │                    │                     │
│                      │              ▼                    │                     │
│                      │  ┌──────────────────────────┐     │                     │
│                      │  │    Contract Engine       │     │                     │
│                      │  └──────────────────────────┘     │                     │
│                      │              │                    │                     │
│                      │              ▼                    │                     │
│                      │  ┌──────────────────────────┐     │                     │
│                      │  │     Settlement           │◀────┼─── 💳 AP2 HERE     │
│                      │  └──────────────────────────┘     │                     │
│                      │                                   │                     │
│                      └───────────────────────────────────┘                     │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Integration Points

AP2 integrates with AEX at the **Settlement** phase. When a contract is completed:

1. **Contract Engine** marks the contract as `COMPLETED`
2. **Settlement** service receives the completion event
3. **Settlement** initiates AP2 payment flow:
   - Creates PaymentMandate from contract details
   - Communicates with Credentials Provider
   - Processes payment through Merchant Payment Processor
4. **Settlement** records the transaction in the ledger

### 2.3 Proposed Architecture with AP2

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                      AEX + AP2 INTEGRATED ARCHITECTURE                          │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  Consumer Side                    AEX Platform                   Provider Side  │
│  ─────────────                   ────────────                   ─────────────   │
│                                                                                 │
│  ┌──────────────┐                                              ┌──────────────┐ │
│  │   Consumer   │                                              │   Provider   │ │
│  │    Agent     │                                              │    Agent     │ │
│  └──────┬───────┘                                              └──────┬───────┘ │
│         │                                                             │         │
│         │                  ┌─────────────────────┐                    │         │
│         │                  │    AEX Gateway      │                    │         │
│         └─────────────────▶│   (A2A Endpoint)    │◀───────────────────┘         │
│                            └──────────┬──────────┘                              │
│                                       │                                         │
│         ┌─────────────────────────────┼─────────────────────────────┐           │
│         │                             ▼                             │           │
│         │            ┌────────────────────────────────┐             │           │
│         │            │        Work Publisher          │             │           │
│         │            └────────────────┬───────────────┘             │           │
│         │                             │                             │           │
│         │                             ▼                             │           │
│         │            ┌────────────────────────────────┐             │           │
│         │            │   Bid Gateway ──▶ Evaluator    │             │           │
│         │            └────────────────┬───────────────┘             │           │
│         │                             │                             │           │
│         │                             ▼                             │           │
│         │            ┌────────────────────────────────┐             │           │
│         │            │      Contract Engine           │             │           │
│         │            └────────────────┬───────────────┘             │           │
│         │                             │                             │           │
│         │                             ▼                             │           │
│         │  ┌─────────────────────────────────────────────────────┐  │           │
│         │  │                   SETTLEMENT                        │  │           │
│         │  │  ┌───────────────────────────────────────────────┐  │  │           │
│         │  │  │            AP2 Payment Handler                │  │  │           │
│         │  │  │  ┌─────────────┐  ┌─────────────┐            │  │  │           │
│         │  │  │  │   Intent    │  │    Cart     │            │  │  │           │
│         │  │  │  │  Mandate    │  │   Mandate   │            │  │  │           │
│         │  │  │  │  Generator  │  │   Handler   │            │  │  │           │
│         │  │  │  └─────────────┘  └─────────────┘            │  │  │           │
│         │  │  │  ┌─────────────┐  ┌─────────────┐            │  │  │           │
│         │  │  │  │  Payment    │  │  Payment    │            │  │  │           │
│         │  │  │  │  Mandate    │  │  Processor  │            │  │  │           │
│         │  │  │  │  Creator    │  │  Client     │            │  │  │           │
│         │  │  │  └─────────────┘  └─────────────┘            │  │  │           │
│         │  │  └───────────────────────────────────────────────┘  │  │           │
│         │  └─────────────────────────────────────────────────────┘  │           │
│         │                             │                             │           │
│         │                             ▼                             │           │
│         │  ┌─────────────────────────────────────────────────────┐  │           │
│         │  │              Credentials Provider                   │  │           │
│         │  │         (External AP2-compliant service)            │  │           │
│         │  └─────────────────────────────────────────────────────┘  │           │
│         │                                                           │           │
│         └───────────────────────────────────────────────────────────┘           │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 2.4 New Components

#### 2.4.1 AP2 Payment Handler (in Settlement Service)

```go
// AP2 Payment Handler - integrates with Settlement service
type AP2PaymentHandler struct {
    credentialsProvider CredentialsProviderClient
    mandateStore        MandateStore
    paymentProcessor    PaymentProcessorClient
}

// ProcessContractPayment handles payment for a completed contract
func (h *AP2PaymentHandler) ProcessContractPayment(ctx context.Context, contract *Contract) (*PaymentResult, error) {
    // 1. Generate IntentMandate from contract details
    intentMandate := h.createIntentMandate(contract)

    // 2. Create CartMandate with provider as merchant
    cartMandate := h.createCartMandate(contract, intentMandate)

    // 3. Get payment methods from Credentials Provider
    paymentMethods, err := h.credentialsProvider.GetPaymentMethods(ctx, contract.ConsumerID)

    // 4. Create PaymentMandate
    paymentMandate := h.createPaymentMandate(cartMandate, paymentMethods[0])

    // 5. Process payment
    result, err := h.paymentProcessor.ProcessPayment(ctx, paymentMandate)

    // 6. Record in ledger
    h.recordTransaction(contract, result)

    return result, nil
}
```

#### 2.4.2 Credentials Provider Agent

A new AEX service that implements the AP2 `credentials-provider` role:

```go
// CredentialsProviderService implements AP2 Credentials Provider
type CredentialsProviderService struct {
    walletStore    WalletStore
    tokenizer      PaymentTokenizer
    a2aServer      *a2a.Server
}

// Skills exposed via A2A
// - get_payment_methods: Returns available payment methods for a user
// - get_payment_token: Returns a tokenized payment credential
// - process_payment: Executes the payment
```

#### 2.4.3 AP2 Extension for AEX Agents

Update AEX agent cards to declare AP2 roles:

```json
{
  "name": "AEX-Provider-Registry",
  "capabilities": {
    "extensions": [
      {
        "uri": "https://github.com/google-agentic-commerce/ap2/tree/v0.1",
        "params": {
          "roles": ["merchant"]
        }
      }
    ]
  }
}
```

### 2.5 Payment Flow in AEX with AP2

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    AEX + AP2 PAYMENT FLOW                                       │
└─────────────────────────────────────────────────────────────────────────────────┘

Consumer Agent          AEX Platform              Provider Agent       CP Agent
     │                      │                          │                   │
     │  1. Publish WorkSpec │                          │                   │
     │─────────────────────▶│                          │                   │
     │                      │                          │                   │
     │                      │  2. Broadcast work       │                   │
     │                      │─────────────────────────▶│                   │
     │                      │                          │                   │
     │                      │  3. Submit bid           │                   │
     │                      │◀─────────────────────────│                   │
     │                      │                          │                   │
     │                      │  4. Award contract       │                   │
     │                      │─────────────────────────▶│                   │
     │                      │                          │                   │
     │                      │  5. Execute work (A2A)   │                   │
     │◀─────────────────────┼─────────────────────────▶│                   │
     │                      │                          │                   │
     │                      │  6. Report completion    │                   │
     │                      │◀─────────────────────────│                   │
     │                      │                          │                   │
     │                      │                          │                   │
     │                      │═══════════════════════════════════════════════│
     │                      │         AP2 PAYMENT PHASE                    │
     │                      │═══════════════════════════════════════════════│
     │                      │                          │                   │
     │                      │  7. Create IntentMandate │                   │
     │                      │     (from contract)      │                   │
     │                      │                          │                   │
     │                      │  8. Get payment methods  │                   │
     │                      │─────────────────────────────────────────────▶│
     │                      │◀─────────────────────────────────────────────│
     │                      │  9. {payment methods}    │                   │
     │                      │                          │                   │
     │                      │  10. Create CartMandate  │                   │
     │                      │      (Provider signs)    │                   │
     │                      │◀─────────────────────────│                   │
     │                      │                          │                   │
     │ 11. Confirm payment  │                          │                   │
     │◀─────────────────────│                          │                   │
     │─────────────────────▶│                          │                   │
     │ 12. User approves    │                          │                   │
     │                      │                          │                   │
     │                      │  13. Create PaymentMandate                   │
     │                      │      + Request payment   │                   │
     │                      │─────────────────────────────────────────────▶│
     │                      │                          │                   │
     │                      │  14. Process & confirm   │                   │
     │                      │◀─────────────────────────────────────────────│
     │                      │                          │                   │
     │                      │  15. Update ledger       │                   │
     │                      │─────────────────────────▶│                   │
     │                      │                          │                   │
     │ 16. Payment complete │                          │                   │
     │◀─────────────────────│                          │                   │
     │                      │                          │                   │
```

### 2.6 Data Mapping: AEX Contract to AP2 Mandates

| AEX Contract Field | AP2 Mandate Field | Notes |
|--------------------|-------------------|-------|
| `contract.id` | `payment_details_id` | Unique identifier |
| `contract.consumer_id` | `payer` | User identifier |
| `contract.provider_id` | `merchant_agent` | Provider as merchant |
| `contract.work_spec.description` | `natural_language_description` | Intent description |
| `contract.bid.price` | `payment_details_total.amount` | Payment amount |
| `contract.bid.currency` | `payment_details_total.currency` | Currency (USD) |
| `contract.work_spec.category` | Intent constraints | Category filtering |

---

## Part 3: Implementation Roadmap

### Phase 1: Foundation (Week 1-2)
- [ ] Add AP2 types package to AEX
- [ ] Implement IntentMandate generation from contracts
- [ ] Implement CartMandate creation
- [ ] Add PaymentMandate generation

### Phase 2: Credentials Provider (Week 3-4)
- [ ] Create mock Credentials Provider service
- [ ] Implement A2A endpoint for payment methods
- [ ] Add payment tokenization (mock)
- [ ] Integrate with Settlement service

### Phase 3: Settlement Integration (Week 5-6)
- [ ] Modify Settlement to use AP2 payment handler
- [ ] Add payment confirmation flow
- [ ] Implement ledger recording with AP2 data
- [ ] Add dispute evidence generation

### Phase 4: Demo & Testing (Week 7-8)
- [ ] Create end-to-end demo with payment flow
- [ ] Add AP2 extension to demo agents
- [ ] Write integration tests
- [ ] Documentation and examples

---

## Part 4: Security Considerations

### 4.1 Cryptographic Requirements
- User authorization requires hardware-backed device keys
- Merchant signatures use RSA/ECDSA with key rotation
- All mandates are tamper-evident (JWT format)

### 4.2 Trust Model
- Credentials Provider must be in trusted registry
- Provider agents must declare AP2 merchant role
- Consumer agents must have user authorization

### 4.3 Dispute Resolution
AP2 mandates serve as evidence in disputes:
- **IntentMandate**: Proves user's original intent
- **CartMandate**: Proves merchant's commitment
- **PaymentMandate**: Proves payment execution

---

## Part 5: Benefits of Integration

| Benefit | Description |
|---------|-------------|
| **Secure Payments** | Cryptographic proof of all transactions |
| **Clear Accountability** | Non-repudiable evidence chain |
| **Interoperability** | Works with any AP2-compliant agent |
| **User Control** | Users approve all payments |
| **Dispute Resolution** | Built-in evidence for chargebacks |
| **Future-Proof** | Open standard with industry backing |

---

## Appendix A: AP2 Message Examples

### A.1 IntentMandate for Contract Work
```json
{
  "ap2.mandates.IntentMandate": {
    "user_cart_confirmation_required": true,
    "natural_language_description": "Legal contract review for employment agreement",
    "merchants": ["legal-agent-a.aex.local"],
    "requires_refundability": false,
    "intent_expiry": "2026-01-20T00:00:00Z"
  }
}
```

### A.2 CartMandate for Completed Work
```json
{
  "ap2.mandates.CartMandate": {
    "contents": {
      "id": "contract_abc123",
      "user_cart_confirmation_required": true,
      "payment_request": {
        "method_data": [{"supported_methods": "CARD"}],
        "details": {
          "id": "contract_abc123",
          "display_items": [
            {
              "label": "Contract Review - Employment Agreement",
              "amount": {"currency": "USD", "value": 150.00}
            }
          ],
          "total": {
            "label": "Total",
            "amount": {"currency": "USD", "value": 150.00}
          }
        }
      },
      "merchant_name": "Legal Agent A (Budget Legal)"
    },
    "merchant_authorization": "eyJhbGciOiJSUzI1NiI..."
  }
}
```

---

## Appendix B: References

- [AP2 Specification](https://github.com/google-agentic-commerce/AP2)
- [A2A Protocol](https://a2a-protocol.org/)
- [W3C Payment Request API](https://www.w3.org/TR/payment-request/)
- [AEX Architecture](../README.md)

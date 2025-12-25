// MongoDB Initialization Script for Agent Exchange
// This script creates databases and indexes for all services

// Switch to the main database
db = db.getSiblingDB('aex');

print('Creating Agent Exchange databases and collections...');

// Work Publisher Collections
db.createCollection('work_specs');
db.work_specs.createIndex({ consumer_id: 1, created_at: -1 });
db.work_specs.createIndex({ state: 1 });
db.work_specs.createIndex({ category: 1 });
print('✓ work_specs collection created');

// Settlement Collections
db.createCollection('executions');
db.executions.createIndex({ consumer_id: 1, created_at: -1 });
db.executions.createIndex({ provider_id: 1, created_at: -1 });
db.executions.createIndex({ domain: 1, created_at: -1 });
db.executions.createIndex({ contract_id: 1 }, { unique: true });
print('✓ executions collection created');

db.createCollection('ledger_entries');
db.ledger_entries.createIndex({ tenant_id: 1, created_at: -1 });
print('✓ ledger_entries collection created');

db.createCollection('tenant_balances');
// No indexes needed, _id is tenant_id
print('✓ tenant_balances collection created');

db.createCollection('transactions');
db.transactions.createIndex({ tenant_id: 1, created_at: -1 });
print('✓ transactions collection created');

// Bid Gateway Collections
db.createCollection('bids');
db.bids.createIndex({ work_id: 1, received_at: -1 });
db.bids.createIndex({ provider_id: 1 });
print('✓ bids collection created');

// Bid Evaluator Collections
db.createCollection('evaluations');
db.evaluations.createIndex({ work_id: 1 });
db.evaluations.createIndex({ created_at: -1 });
print('✓ evaluations collection created');

// Contract Engine Collections
db.createCollection('contracts');
db.contracts.createIndex({ work_id: 1 });
db.contracts.createIndex({ provider_id: 1 });
db.contracts.createIndex({ consumer_id: 1 });
db.contracts.createIndex({ status: 1 });
print('✓ contracts collection created');

// Provider Registry Collections
db.createCollection('providers');
db.providers.createIndex({ provider_id: 1 }, { unique: true });
db.providers.createIndex({ status: 1 });
print('✓ providers collection created');

db.createCollection('subscriptions');
db.subscriptions.createIndex({ provider_id: 1 });
db.subscriptions.createIndex({ category: 1 });
print('✓ subscriptions collection created');

// Trust Broker Collections
db.createCollection('trust_scores');
db.trust_scores.createIndex({ provider_id: 1, agent_id: 1 }, { unique: true });
db.trust_scores.createIndex({ tier: 1 });
print('✓ trust_scores collection created');

db.createCollection('trust_events');
db.trust_events.createIndex({ provider_id: 1, created_at: -1 });
db.trust_events.createIndex({ agent_id: 1, created_at: -1 });
print('✓ trust_events collection created');

// Identity Collections
db.createCollection('tenants');
db.tenants.createIndex({ external_id: 1 }, { unique: true });
db.tenants.createIndex({ type: 1 });
print('✓ tenants collection created');

db.createCollection('api_keys');
db.api_keys.createIndex({ tenant_id: 1 });
db.api_keys.createIndex({ key_hash: 1 }, { unique: true });
db.api_keys.createIndex({ status: 1 });
print('✓ api_keys collection created');

// Create sample data for testing (optional)
print('\nCreating sample test data...');

// Sample tenant
db.tenants.insertOne({
  _id: 'tenant_test001',
  external_id: 'test-consumer-001',
  name: 'Test Consumer',
  type: 'REQUESTOR',
  created_at: new Date(),
  updated_at: new Date()
});

db.tenants.insertOne({
  _id: 'tenant_test002',
  external_id: 'test-provider-001',
  name: 'Test Provider',
  type: 'PROVIDER',
  created_at: new Date(),
  updated_at: new Date()
});

// Sample balances
db.tenant_balances.insertOne({
  _id: 'tenant_test001',
  balance: '1000.00',
  currency: 'USD',
  last_updated: new Date()
});

db.tenant_balances.insertOne({
  _id: 'tenant_test002',
  balance: '0.00',
  currency: 'USD',
  last_updated: new Date()
});

print('✓ Sample test data created');

print('\n✅ MongoDB initialization complete!');
print('\nDatabase: aex');
print('Collections created: ' + db.getCollectionNames().length);

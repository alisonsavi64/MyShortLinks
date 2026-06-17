CREATE KEYSPACE IF NOT EXISTS myshortlinks
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 2};
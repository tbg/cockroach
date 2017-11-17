SET CLUSTER SETTING server.remote_debugging.mode="any"; SET CLUSTER SETTING trace.debug.enable="false"; SET CLUSTER SETTING cluster.organization = "Cockroach Labs - Production Testing"; SET CLUSTER SETTING kv.bulk_io_write.max_rate = "31457280"; SET CLUSTER SETTING enterprise.license = "crl-0-EJL04ukFGAEiI0NvY2tyb2FjaCBMYWJzIC0gUHJvZHVjdGlvbiBUZXN0aW5n";
CREATE DATABASE tpch;
RESTORE tpch.lineitem FROM 'azure://backup-test/benchmarks/tpch/scalefactor-5?AZURE_ACCOUNT_NAME=cockroachbackuptest&AZURE_ACCOUNT_KEY=FzWMkVNcHorQ2IPG5il987GEmEzSxmV8WxXOZHzKV4XfvNTXHEDdBrsAIMB2/UutSPhhNQwVt9zs4dVXD/6w/w=='




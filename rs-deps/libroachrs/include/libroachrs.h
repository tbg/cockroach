DBStatus dbengine_open(DBEngine **ptr, char *dir);
DBStatus dbengine_close(DBEngine *dbe);
void dbengine_put(DBEngine *dbe, char *k, char *v);
DBStatus dbengine_get(DBEngine *dbe, DBKey k, DBString *value);
void from_go(char *ptr, int len);

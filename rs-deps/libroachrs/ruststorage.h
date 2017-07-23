int put(char *k, char *v);
int has(char *name);
void destroy();

void* dbengine_open(char *dir);
void dbengine_put(void *dbe, char *k, char *v);
char* dbengine_get(void *dbe, char *k);
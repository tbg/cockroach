#include <stdlib.h>

// A DBSlice contains read-only data that does not need to be freed.
typedef struct {
  char* data;
  int len;
} DBSlice;

// A DBString is structurally identical to a DBSlice, but the data it
// contains must be freed via a call to free().
typedef struct {
  char* data;
  int len;
} DBString;

// A DBStatus is an alias for DBString and is used to indicate that
// the return value indicates the success or failure of an
// operation. If DBStatus.data == NULL the operation succeeded.
typedef DBString DBStatus;

typedef struct {
  DBSlice key;
  int64_t wall_time;
  int32_t logical;
} DBKey;

typedef struct DBEngine DBEngine;


int put(char *k, char *v);
int has(char *name);
void destroy();

DBStatus dbengine_open(DBEngine **ptr, char *dir);
void dbengine_put(DBEngine *dbe, char *k, char *v);
char* dbengine_get(DBEngine *dbe, char *k);
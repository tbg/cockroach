#include <string>
#include <cryptopp/filters.h>
#include <cryptopp/hex.h>
#include <cryptopp/osrng.h>
#include <cryptopp/sha.h>

std::string Foo;

#ifdef __cplusplus
bool FooAESNI();
#endif

#include "foo.h"

extern "C" {
    bool FooAESNI() { return CryptoPP::UsesAESNI(); }
};

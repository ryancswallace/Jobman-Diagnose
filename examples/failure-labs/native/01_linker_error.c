#include <stddef.h>

extern int acme_rules_initialize(const char *configuration);

int main(void) {
    return acme_rules_initialize("production");
}

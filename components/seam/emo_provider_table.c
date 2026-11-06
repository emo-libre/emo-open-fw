#include <string.h>
#include "emo_seam.h"

static int table_find(void *ctx, const char *name, emo_anim_t *out)
{
    const emo_table_provider_t *t = (const emo_table_provider_t *)ctx;
    for (size_t i = 0; i < t->count; i++) {
        if (t->table[i].name && strcmp(t->table[i].name, name) == 0) {
            *out = t->table[i];
            return EMO_OK;
        }
    }
    return EMO_ERR_NOT_FOUND;
}

void emo_table_provider_init(emo_asset_provider_t *p, emo_table_provider_t *state,
                             const emo_anim_t *table, size_t count)
{
    state->table = table;
    state->count = count;
    p->find = table_find;
    p->ctx = state;
}

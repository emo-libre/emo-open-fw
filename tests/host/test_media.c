#include "test_util.h"
#include "k210_media.h"

int main(void)
{
    char buf[1200];
    k210_playlist_t pl;
    size_t n;
    const char *s;

    /* empty playlist */
    CHECK_EQ(k210_playlist_init(&pl, buf, sizeof buf, 0), 0);
    s = k210_playlist_finish(&pl, &n);
    CHECK(s != NULL);
    CHECK(strcmp(s, "[0,[]]") == 0);
    CHECK_EQ(n, 6);

    /* one clip, one sound (synthetic names) */
    CHECK_EQ(k210_playlist_init(&pl, buf, sizeof buf, 0), 0);
    k210_sound_t snd[] = { { 0, "/test/demo_a.mp3" } };
    CHECK_EQ(k210_playlist_add_clip(&pl, "/test/demo_a.avi", snd, 1), 0);
    s = k210_playlist_finish(&pl, &n);
    CHECK(s != NULL);
    CHECK(strcmp(s, "[0,[[\"/test/demo_a.avi\",[[0,\"/test/demo_a.mp3\"]]]]]") == 0);
    CHECK_EQ(n, strlen(s));

    /* loop flag, two clips, one without sound, timed second sound */
    CHECK_EQ(k210_playlist_init(&pl, buf, sizeof buf, 1), 0);
    k210_sound_t two[] = { { 0, "/spiffs/mp3/demo_b.mp3" }, { 1500, "/spiffs/mp3/demo_c.mp3" } };
    CHECK_EQ(k210_playlist_add_clip(&pl, "/spiffs/avi/demo_b.avi", two, 2), 0);
    CHECK_EQ(k210_playlist_add_clip(&pl, "/spiffs/avi/demo_d.avi", NULL, 0), 0);
    s = k210_playlist_finish(&pl, &n);
    CHECK(s != NULL);
    CHECK(strcmp(s, "[1,[[\"/spiffs/avi/demo_b.avi\",[[0,\"/spiffs/mp3/demo_b.mp3\"],[1500,\"/spiffs/mp3/demo_c.mp3\"]]],"
                    "[\"/spiffs/avi/demo_d.avi\",[]]]]") == 0);

    /* unsafe paths poison the playlist */
    const char *bad[] = { "relative.avi", "/a/../b.avi", "/with\"quote.avi", "/back\\slash.avi", "/sp ace.avi", "/", "" };
    for (unsigned i = 0; i < sizeof bad / sizeof bad[0]; i++) {
        CHECK_EQ(k210_playlist_init(&pl, buf, sizeof buf, 0), 0);
        CHECK_EQ(k210_playlist_add_clip(&pl, bad[i], NULL, 0), -1);
        CHECK(k210_playlist_finish(&pl, &n) == NULL);
    }
    CHECK_EQ(k210_playlist_init(&pl, buf, sizeof buf, 0), 0);
    k210_sound_t badsnd[] = { { 0, "no-slash.mp3" } };
    CHECK_EQ(k210_playlist_add_clip(&pl, "/test/ok.avi", badsnd, 1), -1);
    CHECK(k210_playlist_finish(&pl, &n) == NULL);

    /* size ceiling: body + NUL never exceeds K210_PLAYLIST_SAFE_MAX, even with a bigger buffer */
    char big[4096];
    CHECK_EQ(k210_playlist_init(&pl, big, sizeof big, 0), 0);
    int accepted = 0;
    for (int i = 0; i < 200; i++) {
        if (k210_playlist_add_clip(&pl, "/test/demo_long_name_for_size_test.avi", NULL, 0) == 0) accepted++;
        else break;
    }
    CHECK(accepted > 5 && accepted < 200);
    CHECK(k210_playlist_finish(&pl, &n) == NULL);              /* poisoned by the overflow */
    /* fill to just under the ceiling and succeed */
    CHECK_EQ(k210_playlist_init(&pl, big, sizeof big, 0), 0);
    for (int i = 0; i < accepted; i++) {
        CHECK_EQ(k210_playlist_add_clip(&pl, "/test/demo_long_name_for_size_test.avi", NULL, 0), 0);
    }
    s = k210_playlist_finish(&pl, &n);
    CHECK(s != NULL);
    if (s) CHECK(n + 1 <= K210_PLAYLIST_SAFE_MAX);

    /* too-small buffer */
    char tiny[8];
    CHECK_EQ(k210_playlist_init(&pl, tiny, sizeof tiny, 0), -1);

    /* validators */
    CHECK(k210_media_name_ok("demo_a"));
    CHECK(!k210_media_name_ok(""));
    CHECK(!k210_media_name_ok(NULL));
    CHECK(!k210_media_name_ok("a/b"));
    CHECK(!k210_media_name_ok("a..b"));
    CHECK(!k210_media_name_ok("a b"));
    CHECK(!k210_media_name_ok("quo\"te"));
    CHECK(k210_media_path_ok("/test/demo_a.avi"));
    CHECK(!k210_media_path_ok("test/demo_a.avi"));
    CHECK(!k210_media_path_ok(NULL));

    TEST_DONE();
}

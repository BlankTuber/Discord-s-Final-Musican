#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <dirent.h>
#include <sqlite3.h>
#include <time.h>
#include <unistd.h>

#define MAX_SQL_LENGTH 1024
#define MAX_PATH_LENGTH 512
#define MAX_SONGS 500

typedef struct {
    int id;
    char file_path[MAX_PATH_LENGTH];
    char title[256];
    int play_count;
    long last_played;
} Song;

int initialize_db(sqlite3 **db, const char *db_path) {
    int rc = sqlite3_open(db_path, db);
    
    if (rc != SQLITE_OK) {
        fprintf(stderr, "Cannot open database: %s\n", sqlite3_errmsg(*db));
        sqlite3_close(*db);
        return 1;
    }
    
    return 0;
}

int get_song_count(sqlite3 *db) {
    sqlite3_stmt *stmt;
    char sql[MAX_SQL_LENGTH] = "SELECT COUNT(*) FROM songs";
    int count = 0;
    
    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error: %s\n", sqlite3_errmsg(db));
        return -1;
    }
    
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        count = sqlite3_column_int(stmt, 0);
    }
    
    sqlite3_finalize(stmt);
    return count;
}

int get_least_popular_songs(sqlite3 *db, Song songs[], int limit) {
    sqlite3_stmt *stmt;
    char sql[MAX_SQL_LENGTH] = "SELECT id, file_path, title, play_count, last_played FROM songs "
                               "ORDER BY play_count ASC, last_played ASC LIMIT ?";
    int count = 0;
    
    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error: %s\n", sqlite3_errmsg(db));
        return -1;
    }
    
    sqlite3_bind_int(stmt, 1, limit);
    
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        songs[count].id = sqlite3_column_int(stmt, 0);
        
        const char *file_path = (const char *)sqlite3_column_text(stmt, 1);
        if (file_path) {
            strncpy(songs[count].file_path, file_path, MAX_PATH_LENGTH - 1);
            songs[count].file_path[MAX_PATH_LENGTH - 1] = '\0';
        } else {
            songs[count].file_path[0] = '\0';
        }
        
        const char *title = (const char *)sqlite3_column_text(stmt, 2);
        if (title) {
            strncpy(songs[count].title, title, 255);
            songs[count].title[255] = '\0';
        } else {
            songs[count].title[0] = '\0';
        }
        
        songs[count].play_count = sqlite3_column_int(stmt, 3);
        songs[count].last_played = sqlite3_column_int64(stmt, 4);
        
        count++;
    }
    
    sqlite3_finalize(stmt);
    return count;
}

int delete_song_from_db(sqlite3 *db, int song_id) {
    sqlite3_stmt *stmt;
    char sql[MAX_SQL_LENGTH];
    
    snprintf(sql, MAX_SQL_LENGTH, "DELETE FROM playlist_songs WHERE song_id = ?");
    
    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error: %s\n", sqlite3_errmsg(db));
        return 1;
    }
    
    sqlite3_bind_int(stmt, 1, song_id);
    
    if (sqlite3_step(stmt) != SQLITE_DONE) {
        fprintf(stderr, "Execution failed: %s\n", sqlite3_errmsg(db));
        sqlite3_finalize(stmt);
        return 1;
    }
    
    sqlite3_finalize(stmt);
    
    snprintf(sql, MAX_SQL_LENGTH, "DELETE FROM queue_items WHERE song_id = ?");
    
    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error: %s\n", sqlite3_errmsg(db));
        return 1;
    }
    
    sqlite3_bind_int(stmt, 1, song_id);
    
    if (sqlite3_step(stmt) != SQLITE_DONE) {
        fprintf(stderr, "Execution failed: %s\n", sqlite3_errmsg(db));
        sqlite3_finalize(stmt);
        return 1;
    }
    
    sqlite3_finalize(stmt);
    
    snprintf(sql, MAX_SQL_LENGTH, "DELETE FROM songs WHERE id = ?");
    
    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error: %s\n", sqlite3_errmsg(db));
        return 1;
    }
    
    sqlite3_bind_int(stmt, 1, song_id);
    
    if (sqlite3_step(stmt) != SQLITE_DONE) {
        fprintf(stderr, "Execution failed: %s\n", sqlite3_errmsg(db));
        sqlite3_finalize(stmt);
        return 1;
    }
    
    sqlite3_finalize(stmt);
    return 0;
}

int clean_orphaned_files(const char *music_dir, sqlite3 *db) {
    DIR *dir;
    struct dirent *entry;
    int removed = 0;
    sqlite3_stmt *stmt;
    char sql[MAX_SQL_LENGTH];
    
    dir = opendir(music_dir);
    if (dir == NULL) {
        perror("Failed to open directory");
        return -1;
    }
    
    snprintf(sql, MAX_SQL_LENGTH, "SELECT 1 FROM songs WHERE file_path = ? LIMIT 1");
    
    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error: %s\n", sqlite3_errmsg(db));
        closedir(dir);
        return -1;
    }
    
    while ((entry = readdir(dir)) != NULL) {
        if (entry->d_type == DT_REG) { // Regular file
            const char *ext = strrchr(entry->d_name, '.');
            if (ext != NULL && strcmp(ext, ".mp3") == 0) {
                char full_path[MAX_PATH_LENGTH];
                snprintf(full_path, MAX_PATH_LENGTH, "%s/%s", music_dir, entry->d_name);
                
                sqlite3_bind_text(stmt, 1, full_path, -1, SQLITE_STATIC);
                
                int found = (sqlite3_step(stmt) == SQLITE_ROW);
                sqlite3_reset(stmt);
                
                // Also try with just the filename
                if (!found) {
                    sqlite3_bind_text(stmt, 1, entry->d_name, -1, SQLITE_STATIC);
                    found = (sqlite3_step(stmt) == SQLITE_ROW);
                    sqlite3_reset(stmt);
                }
                
                if (!found) {
                    // File not in database, remove it
                    printf("Removing orphaned file: %s\n", full_path);
                    if (remove(full_path) == 0) {
                        removed++;
                    } else {
                        perror("Failed to remove file");
                    }
                }
            }
        }
    }
    
    sqlite3_finalize(stmt);
    closedir(dir);
    
    return removed;
}

int clean_orphaned_entries(const char *music_dir, sqlite3 *db) {
    sqlite3_stmt *stmt;
    char sql[MAX_SQL_LENGTH];
    int removed = 0;
    
    // Get all song entries
    snprintf(sql, MAX_SQL_LENGTH, "SELECT id, file_path, title FROM songs");
    
    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error: %s\n", sqlite3_errmsg(db));
        return -1;
    }
    
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        int song_id = sqlite3_column_int(stmt, 0);
        const char *file_path = (const char *)sqlite3_column_text(stmt, 1);
        const char *title = (const char *)sqlite3_column_text(stmt, 2);
        
        if (!file_path || file_path[0] == '\0') {
            printf("Entry has no file path: %s (ID: %d)\n", title, song_id);
            if (delete_song_from_db(db, song_id) == 0) {
                removed++;
            }
            continue;
        }
        
        // Check if file exists on its own
        if (access(file_path, F_OK) == -1) {
            // Try with music directory prefix
            char full_path[MAX_PATH_LENGTH];
            const char *filename = strrchr(file_path, '/');
            if (filename) {
                filename++; // Skip the slash
                snprintf(full_path, MAX_PATH_LENGTH, "%s/%s", music_dir, filename);
                
                if (access(full_path, F_OK) == -1) {
                    // File doesn't exist, remove the entry
                    printf("Removing orphaned entry: %s (ID: %d, path: %s)\n", 
                           title, song_id, file_path);
                    if (delete_song_from_db(db, song_id) == 0) {
                        removed++;
                    }
                }
            } else {
                // No slash in path, try with music directory
                snprintf(full_path, MAX_PATH_LENGTH, "%s/%s", music_dir, file_path);
                
                if (access(full_path, F_OK) == -1) {
                    // File doesn't exist, remove the entry
                    printf("Removing orphaned entry: %s (ID: %d, path: %s)\n", 
                           title, song_id, file_path);
                    if (delete_song_from_db(db, song_id) == 0) {
                        removed++;
                    }
                }
            }
        }
    }
    
    sqlite3_finalize(stmt);
    return removed;
}

int main(int argc, char *argv[]) {
    sqlite3 *db;
    char *db_path;
    char *music_dir;
    int song_count;
    int songs_to_remove;
    Song *songs_to_delete;
    int deleted = 0;
    
    if (argc != 3) {
        printf("Usage: %s <database_path> <music_directory>\n", argv[0]);
        return EXIT_FAILURE;
    }
    
    db_path = argv[1];
    music_dir = argv[2];
    
    printf("Janitor starting...\n");
    printf("Database path: %s\n", db_path);
    printf("Music directory: %s\n", music_dir);
    
    if (initialize_db(&db, db_path) != 0) {
        return EXIT_FAILURE;
    }
    
    // First clean orphaned database entries
    printf("Checking for orphaned database entries...\n");
    int orphaned_entries = clean_orphaned_entries(music_dir, db);
    if (orphaned_entries > 0) {
        printf("Removed %d orphaned database entries\n", orphaned_entries);
    } else if (orphaned_entries == 0) {
        printf("No orphaned database entries found\n");
    } else {
        printf("Error checking for orphaned database entries\n");
    }
    
    // Then remove orphaned files (files in directory but not in database)
    printf("Checking for orphaned files...\n");
    int orphaned_files = clean_orphaned_files(music_dir, db);
    if (orphaned_files > 0) {
        printf("Removed %d orphaned files\n", orphaned_files);
    } else if (orphaned_files == 0) {
        printf("No orphaned files found\n");
    } else {
        printf("Error checking for orphaned files\n");
    }
    
    // Then check if we need to clean up based on database count
    song_count = get_song_count(db);
    printf("Total songs in database: %d\n", song_count);
    
    if (song_count > MAX_SONGS) {
        songs_to_remove = song_count - MAX_SONGS;
        printf("Need to remove %d songs to stay under the limit\n", songs_to_remove);
        
        songs_to_delete = (Song *)malloc(songs_to_remove * sizeof(Song));
        if (!songs_to_delete) {
            fprintf(stderr, "Memory allocation failed\n");
            sqlite3_close(db);
            return EXIT_FAILURE;
        }
        
        int found = get_least_popular_songs(db, songs_to_delete, songs_to_remove);
        
        for (int i = 0; i < found; i++) {
            printf("Deleting song: %s (plays: %d, last played: %ld)\n", 
                   songs_to_delete[i].title, 
                   songs_to_delete[i].play_count,
                   songs_to_delete[i].last_played);
            
            // Remove file first
            if (access(songs_to_delete[i].file_path, F_OK) != -1) {
                if (remove(songs_to_delete[i].file_path) != 0) {
                    fprintf(stderr, "Failed to delete file: %s\n", songs_to_delete[i].file_path);
                }
            }
            
            // Then remove from database
            if (delete_song_from_db(db, songs_to_delete[i].id) == 0) {
                deleted++;
            }
        }
        
        free(songs_to_delete);
    } else {
        printf("No cleanup needed, song count is under the limit\n");
    }
    
    sqlite3_close(db);
    
    printf("Janitor completed. Deleted %d songs, removed %d orphaned entries, removed %d orphaned files.\n", 
           deleted, orphaned_entries, orphaned_files);
    return EXIT_SUCCESS;
}
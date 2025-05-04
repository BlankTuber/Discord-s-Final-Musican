#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <dirent.h>
#include <sqlite3.h>
#include <time.h>
#include <unistd.h> // For access()
#include <errno.h>  // Include for errno and strerror()

#define MAX_SQL_LENGTH 1024
#define MAX_PATH_LENGTH 512
#define MAX_SONGS 500

typedef struct {
    int id;
    char file_path[MAX_PATH_LENGTH];
    char title[256];
    int play_count;
    long last_played;
    int is_stream; // Added to match the Go schema
} Song;

int initialize_db(sqlite3 **db, const char *db_path) {
    int rc = sqlite3_open(db_path, db);

    if (rc != SQLITE_OK) {
        fprintf(stderr, "Cannot open database: %s\n", sqlite3_errmsg(*db));
        // No need to close *db here if open failed, it's likely NULL or invalid
        return 1;
    }

    // Optional: Increase busy timeout for better concurrency handling
    sqlite3_busy_timeout(*db, 5000); // 5 seconds

    return 0;
}

int get_song_count(sqlite3 *db) {
    sqlite3_stmt *stmt;
    char sql[MAX_SQL_LENGTH] = "SELECT COUNT(*) FROM songs";
    int count = -1; // Indicate error by default

    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error (get_song_count): %s\n", sqlite3_errmsg(db));
        return -1;
    }

    if (sqlite3_step(stmt) == SQLITE_ROW) {
        count = sqlite3_column_int(stmt, 0);
    } else {
        fprintf(stderr, "SQL error (get_song_count step): %s\n", sqlite3_errmsg(db));
    }

    sqlite3_finalize(stmt);
    return count;
}

int get_least_popular_songs(sqlite3 *db, Song songs[], int limit) {
    sqlite3_stmt *stmt;
    // Select streams last
    char sql[MAX_SQL_LENGTH] = "SELECT id, file_path, title, play_count, last_played, is_stream FROM songs "
                               "ORDER BY is_stream ASC, play_count ASC, last_played ASC LIMIT ?";
    int count = 0;

    if (limit <= 0) return 0; // No songs to get

    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error (get_least_popular_songs): %s\n", sqlite3_errmsg(db));
        return -1; // Indicate error
    }

    sqlite3_bind_int(stmt, 1, limit);

    while (sqlite3_step(stmt) == SQLITE_ROW && count < limit) {
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
        songs[count].last_played = sqlite3_column_int64(stmt, 4); // Use int64 for potentially larger timestamps
        songs[count].is_stream = sqlite3_column_int(stmt, 5); // Read is_stream

        count++;
    }

    // Check for errors during fetching
    int last_rc = sqlite3_errcode(db);
    if (last_rc != SQLITE_OK && last_rc != SQLITE_ROW && last_rc != SQLITE_DONE) {
        fprintf(stderr, "SQL error during fetching (get_least_popular_songs): %s\n", sqlite3_errmsg(db));
        // Depending on severity, might return -1 or rely on caller checking return count
    }


    sqlite3_finalize(stmt);
    return count;
}

int delete_song_from_db(sqlite3 *db, int song_id) {
    sqlite3_stmt *stmt;
    char sql[MAX_SQL_LENGTH];
    int rc;
    int success_count = 0; // Track successful deletions within the transaction

    // Start a transaction for atomic deletion
    rc = sqlite3_exec(db, "BEGIN TRANSACTION", NULL, NULL, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "SQL error (BEGIN TRANSACTION): %s\n", sqlite3_errmsg(db));
        return 1; // Indicate failure
    }

    // Delete from playlist_songs
    snprintf(sql, MAX_SQL_LENGTH, "DELETE FROM playlist_songs WHERE song_id = ?");
    rc = sqlite3_prepare_v2(db, sql, -1, &stmt, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "SQL error (prepare playlist_songs delete): %s\n", sqlite3_errmsg(db));
        sqlite3_exec(db, "ROLLBACK TRANSACTION", NULL, NULL, NULL);
        return 1;
    }
    sqlite3_bind_int(stmt, 1, song_id);
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    if (rc != SQLITE_DONE && rc != SQLITE_OK) { // SQLITE_OK can also indicate success for DML
        fprintf(stderr, "Execution failed (playlist_songs delete, ID %d): %s (RC: %d)\n", song_id, sqlite3_errmsg(db), rc);
        sqlite3_exec(db, "ROLLBACK TRANSACTION", NULL, NULL, NULL);
        return 1;
    }
    success_count++;

    // Delete from queue_items
    snprintf(sql, MAX_SQL_LENGTH, "DELETE FROM queue_items WHERE song_id = ?");
    rc = sqlite3_prepare_v2(db, sql, -1, &stmt, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "SQL error (prepare queue_items delete): %s\n", sqlite3_errmsg(db));
        sqlite3_exec(db, "ROLLBACK TRANSACTION", NULL, NULL, NULL);
        return 1;
    }
    sqlite3_bind_int(stmt, 1, song_id);
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
    if (rc != SQLITE_DONE && rc != SQLITE_OK) {
        fprintf(stderr, "Execution failed (queue_items delete, ID %d): %s (RC: %d)\n", song_id, sqlite3_errmsg(db), rc);
        sqlite3_exec(db, "ROLLBACK TRANSACTION", NULL, NULL, NULL);
        return 1;
    }
    success_count++;


    // Delete from songs
    snprintf(sql, MAX_SQL_LENGTH, "DELETE FROM songs WHERE id = ?");
    rc = sqlite3_prepare_v2(db, sql, -1, &stmt, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "SQL error (prepare songs delete): %s\n", sqlite3_errmsg(db));
        sqlite3_exec(db, "ROLLBACK TRANSACTION", NULL, NULL, NULL);
        return 1;
    }
    sqlite3_bind_int(stmt, 1, song_id);
    rc = sqlite3_step(stmt);
    sqlite3_finalize(stmt);
     if (rc != SQLITE_DONE && rc != SQLITE_OK) {
        fprintf(stderr, "Execution failed (songs delete, ID %d): %s (RC: %d)\n", song_id, sqlite3_errmsg(db), rc);
        sqlite3_exec(db, "ROLLBACK TRANSACTION", NULL, NULL, NULL);
        return 1;
    }
    success_count++;

    // Commit transaction
    rc = sqlite3_exec(db, "COMMIT TRANSACTION", NULL, NULL, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "SQL error (COMMIT TRANSACTION): %d\n", sqlite3_errcode(db));
        // Could potentially try ROLLBACK here if COMMIT fails, but COMMIT failure is often unrecoverable
        return 1; // Indicate failure
    }

    // Successfully reached commit. Number of changes can be checked with sqlite3_total_changes(db),
    // but checking individual step results (SQLITE_DONE/OK) during the transaction is more robust
    // for ensuring each intended delete statement executed without error.
    // If any step failed, the function would have returned early after rolling back.
    return 0; // Indicate success
}

int clean_orphaned_files(const char *music_dir, sqlite3 *db) {
    DIR *dir;
    struct dirent *entry;
    int removed = 0;
    sqlite3_stmt *stmt_check_path;
    sqlite3_stmt *stmt_check_filename;
    char sql_check_path[MAX_SQL_LENGTH];
    char sql_check_filename[MAX_SQL_LENGTH];

    printf("Opening music directory: %s\n", music_dir);
    dir = opendir(music_dir);
    if (dir == NULL) {
        // Use errno and strerror for detailed error
        fprintf(stderr, "Failed to open music directory '%s': %s\n", music_dir, strerror(errno));
        return -1; // Indicate error
    }

    // Prepare statements for checking both full path and filename
    // ONLY check non-stream entries in the database
    snprintf(sql_check_path, MAX_SQL_LENGTH, "SELECT 1 FROM songs WHERE file_path = ? AND is_stream = 0 LIMIT 1");
    snprintf(sql_check_filename, MAX_SQL_LENGTH, "SELECT 1 FROM songs WHERE file_path = ? AND is_stream = 0 LIMIT 1");

    if (sqlite3_prepare_v2(db, sql_check_path, -1, &stmt_check_path, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error (prepare check_path): %s\n", sqlite3_errmsg(db));
        closedir(dir);
        return -1;
    }
     if (sqlite3_prepare_v2(db, sql_check_filename, -1, &stmt_check_filename, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error (prepare check_filename): %s\n", sqlite3_errmsg(db));
        sqlite3_finalize(stmt_check_path); // Clean up already prepared statement
        closedir(dir);
        return -1;
    }

    while ((entry = readdir(dir)) != NULL) {
        // Skip directories, hidden files (starting with .), and special entries "." and ".."
        if (entry->d_type == DT_REG && entry->d_name[0] != '.') {
            const char *ext = strrchr(entry->d_name, '.');
            // Only consider .mp3 files for now as per original logic
            // Add check for ext != NULL to avoid issues with files without extensions
            if (ext != NULL && strcmp(ext, ".mp3") == 0) {
                char full_path[MAX_PATH_LENGTH];
                // Construct the full path by joining music_dir and filename
                snprintf(full_path, MAX_PATH_LENGTH, "%s/%s", music_dir, entry->d_name);

                int found = 0;

                // Check if the full_path exists in the database (for non-stream entries)
                sqlite3_bind_text(stmt_check_path, 1, full_path, -1, SQLITE_STATIC);
                if (sqlite3_step(stmt_check_path) == SQLITE_ROW) {
                    found = 1;
                }
                sqlite3_reset(stmt_check_path); // Reset for the next iteration

                // If not found by full path, check just the filename (for non-stream entries)
                // This accounts for flexibility in how file_path might be stored
                if (!found) {
                     sqlite3_bind_text(stmt_check_filename, 1, entry->d_name, -1, SQLITE_STATIC);
                     if (sqlite3_step(stmt_check_filename) == SQLITE_ROW) {
                         found = 1;
                     }
                     sqlite3_reset(stmt_check_filename); // Reset for the next iteration
                }


                if (!found) {
                    // File not found in database (and is not a stream), remove it
                    printf("Removing orphaned file: %s\n", full_path);
                    if (remove(full_path) == 0) {
                        removed++;
                    } else {
                        // Use strerror to get a system error message
                        fprintf(stderr, "Failed to remove file %s: %s\n", full_path, strerror(errno));
                    }
                }
            }
        }
    }

    sqlite3_finalize(stmt_check_path);
    sqlite3_finalize(stmt_check_filename);
    closedir(dir);

    return removed;
}

int clean_orphaned_entries(const char *music_dir, sqlite3 *db) {
    sqlite3_stmt *stmt;
    char sql[MAX_SQL_LENGTH];
    int removed = 0;

    printf("Starting orphaned database entry check...\n");

    // Get all song entries, including is_stream
    snprintf(sql, MAX_SQL_LENGTH, "SELECT id, file_path, title, is_stream FROM songs");

    if (sqlite3_prepare_v2(db, sql, -1, &stmt, NULL) != SQLITE_OK) {
        fprintf(stderr, "SQL error (prepare orphaned entries): %s\n", sqlite3_errmsg(db));
        return -1; // Indicate error
    }

    while (sqlite3_step(stmt) == SQLITE_ROW) {
        int song_id = sqlite3_column_int(stmt, 0);
        const char *file_path_db = (const char *)sqlite3_column_text(stmt, 1);
        const char *title = (const char *)sqlite3_column_text(stmt, 2);
        int is_stream = sqlite3_column_int(stmt, 3); // Read is_stream

        // If it's a stream, we don't expect a physical file. Skip file existence check and potential deletion.
        if (is_stream) {
            // printf("Skipping file check for stream entry: %s (ID: %d)\n", title ? title : "Unknown", song_id); // Optional debug
            continue;
        }

        // Handle cases where file_path is NULL, empty, or file doesn't exist for non-streams
        int file_exists = 0;
        if (file_path_db && file_path_db[0] != '\0') {
             // Check if file exists at the path stored in the database
            if (access(file_path_db, F_OK) == 0) {
                file_exists = 1;
            } else {
                // If not found at stored path, try constructing path using music_dir
                // This covers cases where file_path might just be the filename relative to music_dir
                char full_path_attempt[MAX_PATH_LENGTH];
                 const char *filename = strrchr(file_path_db, '/'); // Find last slash
                 if (filename) {
                     filename++; // Skip the slash if found
                     snprintf(full_path_attempt, MAX_PATH_LENGTH, "%s/%s", music_dir, filename);
                     if (access(full_path_attempt, F_OK) == 0) {
                          file_exists = 1;
                     }
                 } else {
                     // No slash in file_path_db, assume it might just be the filename
                     snprintf(full_path_attempt, MAX_PATH_LENGTH, "%s/%s", music_dir, file_path_db);
                     if (access(full_path_attempt, F_OK) == 0) {
                          file_exists = 1;
                     }
                 }
            }
        }

        // If file_path is empty/NULL OR file doesn't exist for a non-stream entry
        if (!file_exists) {
            printf("Removing orphaned entry: %s (ID: %d", title ? title : "Unknown", song_id);
             if (file_path_db && file_path_db[0] != '\0') {
                 printf(", path: %s", file_path_db);
             }
             printf(")\n");

            // Attempt to delete the entry from the database
            if (delete_song_from_db(db, song_id) == 0) {
                removed++;
            } else {
                fprintf(stderr, "Failed to delete orphaned entry ID: %d from DB.\n", song_id);
                // Consider if this should halt the process
            }
        }
    }

    sqlite3_finalize(stmt);
     printf("Finished orphaned database entry check.\n");
    return removed;
}


int main(int argc, char *argv[]) {
    sqlite3 *db;
    char *db_path;
    char *music_dir;
    int song_count;
    int songs_to_remove;
    Song *songs_to_delete = NULL; // Initialize to NULL
    int deleted_songs_count = 0;
    int orphaned_entries_count = 0;
    int orphaned_files_count = 0;

    if (argc != 3) {
        printf("Usage: %s <database_path> <music_directory>\n", argv[0]);
        return EXIT_FAILURE;
    }

    db_path = argv[1];
    music_dir = argv[2];

    printf("Janitor starting...\n");
    printf("Database path: %s\n", db_path);
    printf("Music directory: %s\n", music_dir);

    // Check if music directory exists and is accessible
    if (access(music_dir, F_OK | R_OK) == -1) {
        fprintf(stderr, "Error: Music directory '%s' is not accessible or does not exist: %s\n", music_dir, strerror(errno));
        return EXIT_FAILURE;
    }

    if (initialize_db(&db, db_path) != 0) {
        return EXIT_FAILURE;
    }

    // First clean orphaned database entries (which may include references to non-existent files for non-streams)
    // IMPORTANT: Clean entries BEFORE files if you want to remove DB entries for files that are already gone.
    // The updated clean_orphaned_entries handles streams correctly.
    printf("Checking for orphaned database entries...\n");
    orphaned_entries_count = clean_orphaned_entries(music_dir, db);
    if (orphaned_entries_count > 0) {
        printf("Removed %d orphaned database entries\n", orphaned_entries_count);
    } else if (orphaned_entries_count == 0) {
        printf("No orphaned database entries found\n");
    } else {
        fprintf(stderr, "Error checking for orphaned database entries\n");
        // Depending on requirements, you might exit here or continue. Let's continue.
    }

    // Then remove orphaned files (files in directory but not in database for non-streams)
    printf("Checking for orphaned files...\n");
    orphaned_files_count = clean_orphaned_files(music_dir, db);
    if (orphaned_files_count > 0) {
        printf("Removed %d orphaned files\n", orphaned_files_count);
    } else if (orphaned_files_count == 0) {
        printf("No orphaned files found\n");
    } else {
        fprintf(stderr, "Error checking for orphaned files\n");
         // Depending on requirements, you might exit here or continue. Let's continue.
    }

    // Then check if we need to clean up based on database count
    song_count = get_song_count(db);
     if (song_count == -1) {
         fprintf(stderr, "Error getting song count. Skipping song limit cleanup.\n");
     } else {
         printf("Total songs in database: %d\n", song_count);

         if (song_count > MAX_SONGS) {
             songs_to_remove = song_count - MAX_SONGS;
             printf("Need to remove %d songs to stay under the limit of %d\n", songs_to_remove, MAX_SONGS);

             // Allocate memory for the list of songs to delete
             // Ensure we don't allocate an unreasonable amount if calculation is off
             if (songs_to_remove > 0 && songs_to_remove <= song_count + 100) { // Add a buffer to avoid issues if count is slightly off
                  songs_to_delete = (Song *)malloc(songs_to_remove * sizeof(Song));
                  if (!songs_to_delete) {
                      fprintf(stderr, "Memory allocation failed for songs_to_delete\n");
                      // Continue to cleanup DB connection and exit gracefully
                  } else {
                       int found_to_delete = get_least_popular_songs(db, songs_to_delete, songs_to_remove);

                       if (found_to_delete == -1) {
                           fprintf(stderr, "Error retrieving least popular songs.\n");
                       } else {
                           if (found_to_delete < songs_to_remove) {
                               fprintf(stderr, "Warning: Only found %d songs to delete, less than the requested %d.\n", found_to_delete, songs_to_remove);
                           }

                            for (int i = 0; i < found_to_delete; i++) {
                                printf("Considering deletion of song: %s (ID: %d, plays: %d, last played: %ld, is_stream: %d)\n",
                                    songs_to_delete[i].title[0] != '\0' ? songs_to_delete[i].title : "Unknown Title",
                                    songs_to_delete[i].id,
                                    songs_to_delete[i].play_count,
                                    songs_to_delete[i].last_played,
                                    songs_to_delete[i].is_stream);

                                // ONLY attempt to remove the file from disk if it's NOT a stream and has a file_path
                                if (!songs_to_delete[i].is_stream && songs_to_delete[i].file_path[0] != '\0') {
                                     // Check if file exists before attempting removal
                                     if (access(songs_to_delete[i].file_path, F_OK) != -1) {
                                         printf("Removing file: %s\n", songs_to_delete[i].file_path);
                                         if (remove(songs_to_delete[i].file_path) != 0) {
                                             fprintf(stderr, "Failed to delete file %s: %s\n", songs_to_delete[i].file_path, strerror(errno));
                                             // Decide if DB entry should still be deleted if file removal fails.
                                             // Proceeding with DB deletion is usually desired to keep the DB consistent.
                                         }
                                     } else {
                                          // File path in DB for a non-stream doesn't exist on disk
                                          // This might have been an orphaned entry not cleaned in the first pass,
                                          // or a file that disappeared between the check and here.
                                          fprintf(stderr, "Warning: Non-stream entry '%s' (ID: %d) file_path '%s' does not exist for deletion.\n",
                                               songs_to_delete[i].title[0] != '\0' ? songs_to_delete[i].title : "Unknown Title", songs_to_delete[i].id, songs_to_delete[i].file_path);
                                     }
                                } else if (songs_to_delete[i].is_stream) {
                                    printf("Song is a stream (ID: %d), skipping file removal.\n", songs_to_delete[i].id);
                                } else {
                                     // is_stream is 0 but file_path is empty - should be rare
                                     fprintf(stderr, "Warning: Non-stream entry '%s' (ID: %d) has an empty file_path.\n",
                                               songs_to_delete[i].title[0] != '\0' ? songs_to_delete[i].title : "Unknown Title", songs_to_delete[i].id);
                                }

                                // Always remove from database if selected for song count cleanup
                                printf("Removing entry from database: %s (ID: %d)\n", songs_to_delete[i].title[0] != '\0' ? songs_to_delete[i].title : "Unknown Title", songs_to_delete[i].id);
                                if (delete_song_from_db(db, songs_to_delete[i].id) == 0) {
                                    deleted_songs_count++;
                                    // printf("Successfully deleted database entry ID: %d\n", songs_to_delete[i].id); // Already printed in delete function
                                } else {
                                    fprintf(stderr, "Failed to delete database entry ID: %d.\n", songs_to_delete[i].id);
                                    // If DB deletion fails, the entry persists and might be picked up again.
                                    // This indicates a potentially serious issue.
                                }
                           }
                       }

                       free(songs_to_delete); // Free allocated memory
                       songs_to_delete = NULL; // Set pointer to NULL after freeing
                  }
             } else {
                  // songs_to_remove is 0 or an invalid value
                  printf("Calculated songs to remove is 0 or invalid (%d). No song limit cleanup needed.\n", songs_to_remove);
             }

         } else {
             printf("No cleanup needed for song count, %d is under the limit of %d\n", song_count, MAX_SONGS);
         }
     }


    sqlite3_close(db);

    printf("Janitor completed. Deleted %d songs (due to limit), removed %d orphaned entries, removed %d orphaned files.\n",
           deleted_songs_count, orphaned_entries_count, orphaned_files_count);
    return EXIT_SUCCESS;
}
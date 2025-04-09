#include <stdio.h>
#include <stdlib.h>
#include <dirent.h>
#include <string.h>

int filter (const struct dirent *name){
    const char *extension = strrchr(name->d_name, '.');
    if (extension != NULL) {
        return (0 == strcmp(extension, ".mp3"));
    }
    return 0;
}

int main(int argc, char *argv[]) {
    struct dirent **namelist;
    int n;

    if (argc != 2){
        printf("Usage: %s path\n", argv[0]);
        return EXIT_FAILURE;
    }

    n = scandir(argv[1], &namelist, filter, alphasort);
    if (n == -1){
        perror("Scandir");
        exit(EXIT_FAILURE);
    }
    int pathLength = strlen(argv[1]);

    for (int i = 0; i < n; i++) {
        char *fullpath = malloc(pathLength + 1 + strlen(namelist[i]->d_name) + 1);
        if (fullpath == NULL) {
            perror("Memory allocation failed");
            exit(EXIT_FAILURE);
        }

        strcpy(fullpath, argv[1]);

        if (argv[1][pathLength-1] != '/') {
            strcat(fullpath, "/");
        }

        strcat(fullpath, namelist[i]->d_name);
        remove(fullpath);

        free(fullpath);
        free(namelist[i]);
    }

    free(namelist);
    
    exit(EXIT_SUCCESS);
}
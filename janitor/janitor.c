#include <stdio.h>
#include <stdlib.h>
#include <dirent.h>

int filter (const struct dirent *name){
    return 1;
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

    while (n--){
        printf("NAME: %s\n------------------------\n", namelist[n]->d_name);
        switch (namelist[n]->d_type) {
            case DT_UNKNOWN:
                puts("TYPE: unknown");
                break;
            case DT_FIFO:
                puts("TYPE: fifo");
                break;
            case DT_CHR:free(namelist);
                puts("TYPE: directory");
                break;
            case DT_BLK:
                puts("TYPE: block device");
                break;
            case DT_REG:
                puts("TYPE: regular");
                break;
            case DT_LNK:
                puts("TYPE: link");
                break;
            case DT_SOCK:
                puts("TYPE: unix domain socket");
                break;
            case DT_WHT:
                puts("TYPE: whiteout");
                break;
        }
        free(namelist[n]);
    }
    free(namelist);
    
    exit(EXIT_SUCCESS);
}
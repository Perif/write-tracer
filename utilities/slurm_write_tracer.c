/*
 * Slurm SPANK plugin for write-tracer
 *
 * This plugin automatically registers Slurm tasks with the write-tracer REST API
 * upon initialization and unregisters them upon exit.
 *
 * It uses libcurl to communicate with the REST API.
 * Configuration is read from /etc/write-tracer/plugin.conf (default).
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <ctype.h>
#include <slurm/spank.h>
#include <curl/curl.h>

SPANK_PLUGIN(write_tracer, 1);

#define DEFAULT_CONFIG_FILE "/etc/write-tracer/plugin.conf"
#define DEFAULT_TRACER_URL "http://localhost:9092"
#define MAX_URL_LEN 256
#define MAX_LINE_LEN 1024

// Configuration structure
typedef struct {
    char tracer_url[MAX_URL_LEN];
} plugin_config_t;

// Global configuration
static plugin_config_t config;

// Flag to track registered PIDs
static __thread int pid_registered = 0;

// Helper to trim whitespace
static char *trim_whitespace(char *str) {
    char *end;
    while(isspace((unsigned char)*str)) str++;
    if(*str == 0) return str;
    end = str + strlen(str) - 1;
    while(end > str && isspace((unsigned char)*end)) end--;
    *(end+1) = 0;
    return str;
}

// Function to read configuration
static void load_config() {
    // Set defaults
    strncpy(config.tracer_url, DEFAULT_TRACER_URL, MAX_URL_LEN - 1);
    config.tracer_url[MAX_URL_LEN - 1] = '\0';

    FILE *fp = fopen(DEFAULT_CONFIG_FILE, "r");
    if (!fp) {
        // It's okay if config file doesn't exist, use defaults
        // slurm_info("write-tracer: Config file not found, using default URL: %s", config.tracer_url);
        return;
    }

    char line[MAX_LINE_LEN];
    while (fgets(line, sizeof(line), fp)) {
        char *key = strtok(line, "=");
        char *val = strtok(NULL, "\n");

        if (key && val) {
            key = trim_whitespace(key);
            val = trim_whitespace(val);

            if (strcmp(key, "TRACER_URL") == 0) {
                strncpy(config.tracer_url, val, MAX_URL_LEN - 1);
                config.tracer_url[MAX_URL_LEN - 1] = '\0';
            }
        }
    }
    fclose(fp);
}

// Helper function to perform curl request
static int send_request(const char *url_path, const char *json_data, const char *method) {
    CURL *curl;
    CURLcode res;
    long response_code;
    char full_url[MAX_URL_LEN * 2];
    struct curl_slist *headers = NULL;

    snprintf(full_url, sizeof(full_url), "%s%s", config.tracer_url, url_path);

    curl = curl_easy_init();
    if (!curl) {
        slurm_error("write-tracer: Failed to initialize curl");
        return -1;
    }

    headers = curl_slist_append(headers, "Content-Type: application/json");
    
    curl_easy_setopt(curl, CURLOPT_URL, full_url);
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_CUSTOMREQUEST, method);
    
    if (json_data) {
        curl_easy_setopt(curl, CURLOPT_POSTFIELDS, json_data);
    }

    // Set timeout to avoid blocking indefinitely
    curl_easy_setopt(curl, CURLOPT_TIMEOUT, 2L); 
    curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT, 1L);

    // Suppress output
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, NULL); // Using default which writes to stdout... wait
    // Actually we should suppress output.
    FILE *devnull = fopen("/dev/null", "w");
    if (devnull) {
        curl_easy_setopt(curl, CURLOPT_WRITEDATA, devnull);
    }

    res = curl_easy_perform(curl);
    
    if (res != CURLE_OK) {
        slurm_error("write-tracer: curl request failed to %s: %s", full_url, curl_easy_strerror(res));
    } else {
        curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &response_code);
        if (response_code >= 400) {
            slurm_error("write-tracer: Server returned error code %ld for %s", response_code, full_url);
            res = CURLE_HTTP_RETURNED_ERROR;
        }
    }

    if (devnull) fclose(devnull);
    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);

    return (res == CURLE_OK) ? 0 : -1;
}

// Called when the plugin is loaded
int slurm_spank_init(spank_t sp, int ac, char **av) {
    load_config();
    return 0;
}

// Called for each task initialization
int slurm_spank_task_init(spank_t sp, int ac, char **av) {
    pid_t pid = getpid();
    char json_payload[64];
    
    snprintf(json_payload, sizeof(json_payload), "{\"pid\": %d}", pid);
    
    if (send_request("/pids", json_payload, "POST") == 0) {
        // slurm_info("write-tracer: Registered PID %d", pid);
        pid_registered = 1;
    } 
    
    return 0; // Always return 0 to allow task to proceed
}

// Called for each task exit
int slurm_spank_task_exit(spank_t sp, int ac, char **av) {
    // Only unregister if the PID was actually registered
    if (pid_registered) {
        pid_t pid = getpid();
        char url_path[64];

        snprintf(url_path, sizeof(url_path), "/pids/%d", pid);
    
        if (send_request(url_path, NULL, "DELETE") == 0) {
            // slurm_info("write-tracer: Unregistered PID %d", pid);
        }
        pid_registered = 0;
    }
    
    return 0;
}

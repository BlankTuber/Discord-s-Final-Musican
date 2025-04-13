import config
import ytdlp_actions as downloader

def main():
    print("Reading configuration...")
    print(f"Downloading files to: {config.DOWNLOAD_PATH}")
    print(f"Using socket at: {config.UDS_LINK}")
    print(f"Allowed origins: {config.ALLOWED_ORIGINS}")
    
    # Test single audio download
    print("\n--- Testing single audio download ---")
    test_url = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
    print(f"Downloading: {test_url}")
    result = downloader.download_audio(test_url, max_duration_seconds=600, max_size_mb=50)
    
    if result:
        print(f"Download successful!")
        print(f"Title: {result['title']}")
        print(f"Saved as: {result['filename']}")
        print(f"Duration: {result['duration']} seconds")
    else:
        print("Download failed or was skipped")
    
    # Test search functionality
    print("\n--- Testing search functionality ---")
    search_term = "lofi beats"
    print(f"Searching for: {search_term}")
    search_results = downloader.search(search_term, limit=3)
    
    if search_results:
        print(f"Found {len(search_results)} results:")
        for i, result in enumerate(search_results, 1):
            print(f"{i}. {result['title']} by {result['uploader']}")
            print(f"   URL: {result['url']}")
            print(f"   Duration: {result['duration']} seconds")
            print()
    else:
        print("Search returned no results")

if __name__ == "__main__":
    main()
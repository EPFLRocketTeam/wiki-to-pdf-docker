from dataclasses import dataclass
from datetime import datetime
from typing import Dict, Optional

@dataclass
class PageMetadata:
    markdown_content: str
    author: str
    date: str
    title: str
    document_id: str
    template: str
    latex_content: Optional[str] = None
    
class PageDataManager:
    def __init__(self):
        self.pages: Dict[str, PageMetadata] = {}
        
    def add_page(self, url: str, markdown_content: str = "", 
                 author: str = "", date: str = "", 
                 title: str = "", document_id: str = "",
                 template: str = "default") -> None:
        """
        Add or update a page's data
        
        Args:
            url: The page URL (used as unique identifier)
            markdown_content: The page's markdown content
            author: The page author
            date: The document date
            title: The document title
            document_id: The document identifier
            template: The template to use for conversion
        """
        # If date is empty, use current date
        if not date:
            date = datetime.now().strftime('%Y-%m-%d')
            
        self.pages[url] = PageMetadata(
            markdown_content=markdown_content,
            author=author,
            date=date,
            title=title,
            document_id=document_id,
            template=template
        )
    
    def update_page(self, url: str, **kwargs) -> bool:
        """
        Update specific fields of a page
        
        Args:
            url: The page URL
            **kwargs: The fields to update and their new values
            
        Returns:
            bool: True if page was updated, False if page not found
        """
        if url not in self.pages:
            return False
            
        page = self.pages[url]
        for key, value in kwargs.items():
            if hasattr(page, key):
                setattr(page, key, value)
        return True
    
    def get_page(self, url: str) -> Optional[PageMetadata]:
        """Get a page's data"""
        return self.pages.get(url)
    
    def get_all_pages(self) -> Dict[str, PageMetadata]:
        """Get all pages' data"""
        return self.pages
    
    def delete_page(self, url: str) -> bool:
        """Delete a page's data"""
        if url in self.pages:
            del self.pages[url]
            return True
        return False
    
    def clear_all(self) -> None:
        """Clear all stored page data"""
        self.pages.clear()

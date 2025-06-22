export namespace backend {
	
	export class EmailCheckResult {
	    account?: models.EmailAccount;
	    new_emails: number;
	    pdfs_found: number;
	    error?: string;
	    success: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EmailCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.account = this.convertValues(source["account"], models.EmailAccount);
	        this.new_emails = source["new_emails"];
	        this.pdfs_found = source["pdfs_found"];
	        this.error = source["error"];
	        this.success = source["success"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GetDownloadTasksResponse {
	    tasks: models.DownloadTask[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new GetDownloadTasksResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tasks = this.convertValues(source["tasks"], models.DownloadTask);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace models {
	
	export class AppConfig {
	    id: number;
	    download_path: string;
	    max_concurrent: number;
	    check_interval: number;
	    auto_check: boolean;
	    minimize_to_tray: boolean;
	    start_minimized: boolean;
	    enable_notification: boolean;
	    theme: string;
	    language: string;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.download_path = source["download_path"];
	        this.max_concurrent = source["max_concurrent"];
	        this.check_interval = source["check_interval"];
	        this.auto_check = source["auto_check"];
	        this.minimize_to_tray = source["minimize_to_tray"];
	        this.start_minimized = source["start_minimized"];
	        this.enable_notification = source["enable_notification"];
	        this.theme = source["theme"];
	        this.language = source["language"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class DownloadStatistics {
	    id: number;
	    date: string;
	    total_downloads: number;
	    success_downloads: number;
	    failed_downloads: number;
	    total_size: number;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadStatistics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.date = source["date"];
	        this.total_downloads = source["total_downloads"];
	        this.success_downloads = source["success_downloads"];
	        this.failed_downloads = source["failed_downloads"];
	        this.total_size = source["total_size"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class EmailAccount {
	    id: number;
	    name: string;
	    email: string;
	    password: string;
	    imap_server: string;
	    imap_port: number;
	    use_ssl: boolean;
	    is_active: boolean;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new EmailAccount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.email = source["email"];
	        this.password = source["password"];
	        this.imap_server = source["imap_server"];
	        this.imap_port = source["imap_port"];
	        this.use_ssl = source["use_ssl"];
	        this.is_active = source["is_active"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class DownloadTask {
	    id: number;
	    email_id: number;
	    email_account: EmailAccount;
	    subject: string;
	    sender: string;
	    file_name: string;
	    file_size: number;
	    downloaded_size: number;
	    status: string;
	    type: string;
	    source: string;
	    local_path: string;
	    error: string;
	    progress: number;
	    speed: string;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email_id = source["email_id"];
	        this.email_account = this.convertValues(source["email_account"], EmailAccount);
	        this.subject = source["subject"];
	        this.sender = source["sender"];
	        this.file_name = source["file_name"];
	        this.file_size = source["file_size"];
	        this.downloaded_size = source["downloaded_size"];
	        this.status = source["status"];
	        this.type = source["type"];
	        this.source = source["source"];
	        this.local_path = source["local_path"];
	        this.error = source["error"];
	        this.progress = source["progress"];
	        this.speed = source["speed"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class EmailMessage {
	    id: number;
	    email_id: number;
	    email_account: EmailAccount;
	    message_id: string;
	    subject: string;
	    sender: string;
	    recipients: string;
	    date: string;
	    has_pdf: boolean;
	    is_processed: boolean;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new EmailMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email_id = source["email_id"];
	        this.email_account = this.convertValues(source["email_account"], EmailAccount);
	        this.message_id = source["message_id"];
	        this.subject = source["subject"];
	        this.sender = source["sender"];
	        this.recipients = source["recipients"];
	        this.date = source["date"];
	        this.has_pdf = source["has_pdf"];
	        this.is_processed = source["is_processed"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

